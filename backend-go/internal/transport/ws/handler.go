// Package ws 实现 WebSocket 传输层，处理 /ws/sessions/:sessionId 连接。
//
// 协议见 SPEC-GO-REWRITE.md §2.4。
// 核心职责：
//   - 升级 HTTP → WebSocket
//   - 解析 ClientCommand（kind 判别）并分发到 SessionService
//   - 推送 ServerEnvelope（事件信封）给客户端
//   - WS close 时 endSession（杀进程 + status=ended）
//
// 并发模型：每个 WS 连接两个 goroutine
//   - readPump：读客户端消息，分发命令
//   - writePump：从订阅 channel 读事件，写回客户端（含 ping/pong）
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/heycode/backend-go/internal/auth"
	"github.com/heycode/backend-go/internal/eventbus"
	"github.com/heycode/backend-go/internal/service"
	"github.com/heycode/backend-go/internal/types"
)

// maxPayload 限制单帧 16MB（§2.4.1）。
const maxPayload = 16 * 1024 * 1024

// 读写超时
const (
	writeWait      = 10 * time.Second // 单次写超时
	pongWait       = 60 * time.Second // 等 pong 超时（客户端 20s 发 ping）
	pingPeriod     = 30 * time.Second // 后端主动 ping 间隔（小于 pongWait）
	readLimit      = maxPayload
)

// upgrader 升级 HTTP → WS。CheckOrigin 放行所有（自托管场景）。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Handler 处理 WS 连接。
type Handler struct {
	svc *service.SessionService
	// authMgr 可为 nil（鉴权未启用）。非 nil 且启用时，Upgrade 前校验 ?token=。
	authMgr *auth.Manager
}

// NewHandler 创建 WS handler。authMgr 可为 nil。
func NewHandler(svc *service.SessionService, authMgr *auth.Manager) *Handler {
	return &Handler{svc: svc, authMgr: authMgr}
}

// ClientCommand 是客户端 → 服务端的消息（§2.4.2，kind 判别）。
type ClientCommand struct {
	Kind              string   `json:"kind"`
	ServerID          string   `json:"serverId,omitempty"`
	Cwd               string   `json:"cwd,omitempty"`
	Cli               string   `json:"cli,omitempty"`
	Prompt            string   `json:"prompt,omitempty"`
	Model             string   `json:"model,omitempty"`
	ResumeCliSessionID string  `json:"resumeCliSessionId,omitempty"`
	AllowedTools      []string `json:"allowedTools,omitempty"`
	SinceEventID      int64    `json:"sinceEventId,omitempty"`
}

// errorFrame 是非信封错误帧（§2.4.3）。
type errorFrame struct {
	Error string `json:"error"`
}

// pongFrame 是心跳响应。
type pongFrame struct {
	Type string `json:"type"`
}

// ServeHTTP 处理 GET /ws/sessions/:sessionId。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	if sessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	// 鉴权：浏览器 WS 不能设 header，统一走 ?token= query param。
	// authMgr 为 nil 或未启用时 VerifyQuery 直接返回 true。
	if h.authMgr != nil && !h.authMgr.VerifyQuery(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "session", sessionID, "error", err)
		return
	}
	conn.SetReadLimit(readLimit)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	c := &connection{
		svc:       h.svc,
		sessionID: sessionID,
		conn:      conn,
		send:      make(chan []byte, 256),
	}

	// 标记是否已通过 session.start 创建会话；
	// 仅在已 start 的情况下，WS close 才调 endSession（避免误杀其它会话）。
	c.started = false

	go c.writePump()
	c.readPump()
}

// connection 封装单个 WS 连接的读写循环。
type connection struct {
	svc       *service.SessionService
	sessionID string
	conn      *websocket.Conn
	send      chan []byte // writePump 消费
	mu        sync.Mutex
	started   bool // 是否已 session.start（close 时据此决定是否 endSession）
	subCancel func() // 事件总线订阅取消函数
}

// readPump 读客户端消息并分发命令。
// 循环退出意味着连接关闭 → 触发 cleanup（取消订阅 + endSession）。
func (c *connection) readPump() {
	defer func() {
		c.cleanup()
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Debug("ws read error", "session", c.sessionID, "error", err)
			}
			return
		}

		// 尝试解析为 ClientCommand（kind 判别）
		var cmd ClientCommand
		if err := json.Unmarshal(message, &cmd); err != nil {
			c.sendError("请求体格式错误: " + err.Error())
			continue
		}

		// 心跳特殊处理（非 ClientCommand，用 type 字段）
		var ping struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(message, &ping) == nil && ping.Type == "ping" {
			c.sendJSON(pongFrame{Type: "pong"})
			continue
		}

		if cmd.Kind == "" {
			c.sendError("无效指令：缺少 kind 字段")
			continue
		}

		c.handleCommand(cmd)
	}
}

// handleCommand 分发 ClientCommand 到对应 service 方法。
func (c *connection) handleCommand(cmd ClientCommand) {
	ctx := context.Background()

	switch cmd.Kind {
	case "session.start":
		c.handleStart(ctx, cmd)

	case "session.send":
		if !c.started {
			c.sendError("会话尚未启动，请先发送 session.start")
			return
		}
		if err := c.svc.SendInput(ctx, c.sessionID, cmd.Prompt); err != nil {
			c.sendError("发送输入失败: " + err.Error())
		}

	case "session.interrupt":
		if !c.started {
			c.sendError("会话尚未启动")
			return
		}
		if err := c.svc.Interrupt(c.sessionID); err != nil {
			c.sendError("中断失败: " + err.Error())
		}

	case "session.end":
		if !c.started {
			c.sendError("会话尚未启动")
			return
		}
		if err := c.svc.EndSession(ctx, c.sessionID); err != nil {
			c.sendError("结束会话失败: " + err.Error())
		}
		c.started = false

	case "session.resync":
		c.handleResync(ctx, cmd)

	default:
		c.sendError("无效指令：未知 kind \"" + cmd.Kind + "\"")
	}
}

// handleStart 处理 session.start。
// 先订阅事件总线（防漏 session.init），再调 StartSession 启动 runner。
func (c *connection) handleStart(ctx context.Context, cmd ClientCommand) {
	if c.started {
		c.sendError("会话已启动，请勿重复 session.start")
		return
	}

	if cmd.Cli == "" {
		c.sendError("session.start 缺少 cli 字段")
		return
	}

	// 先订阅事件总线（用 URL sessionId），确保不漏 session.init
	ch, cancel := c.svc.Subscribe(c.sessionID)

	opts := service.StartSessionOptions{
		TaskID:             nil,
		ServerID:           cmd.ServerID,
		Cwd:                cmd.Cwd,
		Cli:                types.CliKind(cmd.Cli),
		Model:              cmd.Model,
		Prompt:             cmd.Prompt,
		ResumeCliSessionID: cmd.ResumeCliSessionID,
		AllowedTools:       cmd.AllowedTools,
	}

	// StartSession 用 URL sessionId 作为预设 ID
	sid := c.sessionID
	_, err := c.svc.StartSession(ctx, &sid, opts)
	if err != nil {
		cancel()
		c.sendError("启动会话失败: " + err.Error())
		return
	}

	c.mu.Lock()
	c.started = true
	c.subCancel = cancel
	c.mu.Unlock()

	// 启动事件转发 goroutine：订阅 channel → c.send
	go c.forwardEvents(ch)
}

// handleResync 处理 session.resync：回放 eventId > sinceEventId 的历史事件。
func (c *connection) handleResync(ctx context.Context, cmd ClientCommand) {
	envelopes, err := c.svc.GetEvents(ctx, c.sessionID, cmd.SinceEventID)
	if err != nil {
		c.sendError("回放历史事件失败: " + err.Error())
		return
	}
	for _, env := range envelopes {
		c.sendEnvelope(env)
	}
}

// forwardEvents 把订阅 channel 的事件转发到 c.send（供 writePump 写出）。
// 订阅 channel 关闭时退出。
func (c *connection) forwardEvents(ch <-chan *eventbus.Envelope) {
	for env := range ch {
		c.sendEnvelope(env)
	}
}

// sendEnvelope 序列化信封并送入 send channel。
func (c *connection) sendEnvelope(env *eventbus.Envelope) {
	data, err := json.Marshal(env)
	if err != nil {
		slog.Warn("marshal envelope failed", "session", c.sessionID, "error", err)
		return
	}
	select {
	case c.send <- data:
	default:
		// send buffer 满：丢弃（慢消费者；断线重连靠 resync 兜底）
		slog.Warn("ws send buffer full, dropping envelope", "session", c.sessionID, "eventId", env.EventID)
	}
}

// sendError 发送非信封错误帧。
func (c *connection) sendError(msg string) {
	c.sendJSON(errorFrame{Error: msg})
}

// sendJSON 序列化任意消息并送入 send channel。
func (c *connection) sendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// writePump 从 send channel 读消息写回客户端，并周期性 ping。
func (c *connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// send channel 关闭（cleanup 触发）→ 发 close 帧退出
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// cleanup 在 WS 连接关闭时执行：取消订阅 + endSession（§4.5）。
// 仅在已 session.start 的情况下才 endSession，避免误杀。
func (c *connection) cleanup() {
	c.mu.Lock()
	cancel := c.subCancel
	started := c.started
	c.subCancel = nil
	c.started = false
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if started {
		// WS close → endSession（杀进程 + status=ended + cleanup）
		if err := c.svc.EndSession(context.Background(), c.sessionID); err != nil {
			slog.Warn("endSession on ws close failed", "session", c.sessionID, "error", err)
		}
	}

	// 关闭 send channel 让 writePump 退出
	close(c.send)
}
