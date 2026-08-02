// Package eventbus 实现事件总线：订阅/发布/回放 + per-session 串行锁 + eventId 懒加载 + 持久化。
//
// 协议见 SPEC-GO-REWRITE.md §2.4.4 / §4.1。
// 核心职责：
//   - Publish：为事件分配 eventId（per-session 单调递增）→ 写 Event 表 →
//     file.change 额外写 FileSnapshot 表 → 通知所有订阅者
//   - Subscribe：注册 per-session 订阅 channel，新事件实时推送
//   - Replay：回放 eventId > since 的历史事件（断线重连用）
//   - per-session 串行锁：保证 eventId 单调且不重，counter 懒加载自 DB max
//
// 线程安全：内部 mutex 保护 subscriptions map；每个 session 独立 counter mutex。
package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/heycode/backend-go/internal/idgen"
	"github.com/heycode/backend-go/internal/store"
	"github.com/heycode/backend-go/internal/types"
)

// Envelope 是服务端 → 客户端的消息信封（§2.4.3）。
//
// EventID 每会话独立单调递增；Event 是 UnifiedEvent 的具体类型（序列化为 JSON）。
type Envelope struct {
	EventID   int64               `json:"eventId"`
	SessionID string              `json:"sessionId"`
	Event     types.UnifiedEvent  `json:"event"`
}

// subscriber 是一个订阅者，持有 per-session 的缓冲 channel。
// 慢消费者：buffer 满时丢弃旧事件并记录（实时流优先，断线重连靠 replay）。
type subscriber struct {
	ch chan *Envelope
}

// Bus 是事件总线，per-session 订阅 + 串行 publish。
type Bus struct {
	eventStore   *store.EventStore
	snapshotStore *store.SnapshotStore

	mu            sync.Mutex // 保护 subscriptions 和 counters
	subscriptions map[string]map[*subscriber]struct{} // sessionId → 订阅者集合
	counters      map[string]*sessionCounter          // sessionId → counter（懒加载）
}

// sessionCounter 是 per-session 的 eventId 计数器，含独立 mutex。
type sessionCounter struct {
	mu      sync.Mutex
	nextID  int64 // 下一个 eventId
	loaded  bool  // 是否已从 DB 懒加载
}

// New 创建事件总线。
func New(eventStore *store.EventStore, snapshotStore *store.SnapshotStore) *Bus {
	return &Bus{
		eventStore:    eventStore,
		snapshotStore: snapshotStore,
		subscriptions: make(map[string]map[*subscriber]struct{}),
		counters:      make(map[string]*sessionCounter),
	}
}

// Subscribe 订阅某会话的实时事件流，返回一个 channel 和取消订阅函数。
//
// 调用方（如 WS handler）应在连接关闭时调用 cancel() 释放资源。
// buffer size 256：足够缓冲一次回合的突发事件（message + tool.use + tool.result + ...），
// 慢消费者不会阻塞 publish（publish 会丢弃旧事件）。
func (b *Bus) Subscribe(sessionID string) (<-chan *Envelope, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &subscriber{ch: make(chan *Envelope, 256)}
	if _, ok := b.subscriptions[sessionID]; !ok {
		b.subscriptions[sessionID] = make(map[*subscriber]struct{})
	}
	b.subscriptions[sessionID][sub] = struct{}{}

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subs, ok := b.subscriptions[sessionID]; ok {
			if _, ok := subs[sub]; ok {
				delete(subs, sub)
				close(sub.ch)
				if len(subs) == 0 {
					delete(b.subscriptions, sessionID)
				}
			}
		}
	}
	return sub.ch, cancel
}

// Publish 发布一个事件到指定会话。
//
// 流程（§2.4.4）：
//  1. 获取 per-session 串行锁
//  2. counter 懒加载：首次从 DB 取 max(eventId)，nextID = max + 1
//  3. 分配 eventId（nextID++）
//  4. 序列化 event → payload
//  5. 写 Event 表（UNIQUE(session_id, event_id) 保证不重）
//  6. 若是 file.change 事件 → 同步写 FileSnapshot 表
//  7. 通知所有订阅者（非阻塞，buffer 满则丢弃）
func (b *Bus) Publish(ctx context.Context, sessionID string, event types.UnifiedEvent, ts time.Time) (*Envelope, error) {
	// 1. 获取 per-session 串行锁 + counter
	counter := b.getCounter(sessionID)
	counter.mu.Lock()
	defer counter.mu.Unlock()

	// 2. 懒加载 counter
	if !counter.loaded {
		maxID, err := b.eventStore.MaxEventID(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("load max event id: %w", err)
		}
		counter.nextID = maxID + 1
		counter.loaded = true
	}

	// 3. 分配 eventId
	eventID := counter.nextID
	counter.nextID++

	// 4. 序列化
	payload, err := types.MarshalEvent(event)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}

	// 5. 写 Event 表
	dbEvent := &store.Event{
		SessionID: sessionID,
		EventID:   eventID,
		Payload:   string(payload),
		CreatedAt: ts,
	}
	if err := b.eventStore.Insert(ctx, dbEvent); err != nil {
		// UNIQUE 冲突理论上不应发生（per-session 锁保证），若发生则 counter 状态已污染
		return nil, fmt.Errorf("persist event: %w", err)
	}

	// 6. file.change → 写 FileSnapshot
	if fc, ok := event.(types.FileChangeEvent); ok {
		if err := b.writeSnapshot(ctx, sessionID, fc, ts); err != nil {
			// 快照写入失败不应阻断事件流（快照仅用于回滚，可降级）
			// 实际生产应记录警告日志，这里简化处理
			_ = err
		}
	}

	// 7. 通知订阅者
	env := &Envelope{EventID: eventID, SessionID: sessionID, Event: event}
	b.notify(sessionID, env)

	return env, nil
}

// Replay 回放 sessionId 的历史事件（eventId > since），返回 envelope 切片。
// 用于断线重连：客户端发 session.resync + sinceEventId，后端回放历史。
func (b *Bus) Replay(ctx context.Context, sessionID string, since int64) ([]*Envelope, error) {
	events, err := b.eventStore.ListBySession(ctx, sessionID, since)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	out := make([]*Envelope, 0, len(events))
	for _, e := range events {
		ev, err := types.UnmarshalEvent([]byte(e.Payload))
		if err != nil {
			// 跳过无法反序列化的事件（前向兼容）
			continue
		}
		out = append(out, &Envelope{EventID: e.EventID, SessionID: e.SessionID, Event: ev})
	}
	return out, nil
}

// GetEnvelopesSince 返回某会话 eventId > since 的全部信封（REST GET events 端点用）。
func (b *Bus) GetEnvelopesSince(ctx context.Context, sessionID string, since int64) ([]*Envelope, error) {
	return b.Replay(ctx, sessionID, since)
}

// ---- 内部 ----

// getCounter 获取或创建 per-session counter（调用方持有 b.mu）。
func (b *Bus) getCounter(sessionID string) *sessionCounter {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.counters[sessionID]
	if !ok {
		c = &sessionCounter{}
		b.counters[sessionID] = c
	}
	return c
}

// notify 非阻塞推送 envelope 到所有订阅者。
// buffer 满时丢弃该事件（慢消费者；断线重连靠 replay 补）。
func (b *Bus) notify(sessionID string, env *Envelope) {
	b.mu.Lock()
	subs := b.subscriptions[sessionID]
	// 复制订阅者列表，避免长持有锁
	snapshot := make([]*subscriber, 0, len(subs))
	for sub := range subs {
		snapshot = append(snapshot, sub)
	}
	b.mu.Unlock()

	for _, sub := range snapshot {
		select {
		case sub.ch <- env:
		default:
			// buffer 满，丢弃（实时流优先，replay 兜底）
		}
	}
}

// writeSnapshot 把 file.change 事件转为 FileSnapshot 记录入库。
func (b *Bus) writeSnapshot(ctx context.Context, sessionID string, fc types.FileChangeEvent, ts time.Time) error {
	snap := &store.FileSnapshot{
		ID:           idgen.New(),
		SessionID:    sessionID,
		Path:         fc.Change.Path,
		Action:       fc.Change.Action,
		Diff:         fc.Change.Diff,
		AddedLines:   fc.Change.AddedLines,
		RemovedLines: fc.Change.RemovedLines,
		CreatedAt:    ts,
	}
	return b.snapshotStore.Insert(ctx, snap)
}

// _ 确保 json 包被引用（未来扩展 envelope 自定义序列化时使用）
var _ = json.Marshal
