package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/heycode/backend-go/internal/eventbus"
	"github.com/heycode/backend-go/internal/runner"
	"github.com/heycode/backend-go/internal/store"
	"github.com/heycode/backend-go/internal/types"
)

// SessionService 封装会话业务逻辑：REST CRUD + WS 生命周期管理。
//
// 核心流程（§2.8.1）：
//   - StartSession：建 Session(status=running) → 订阅事件总线 → 异步启动 runner
//   - watchSession：监听 session.init（写回 cliSessionId）与 session.end（更新 status）
//   - EndSession：取消 watcher → status=ended → runner.Kill
//
// 依赖方向：service → store + eventbus + runner + types
type SessionService struct {
	store   *store.SessionStore
	bus     *eventbus.Bus
	runner  *runner.Runner
	mockCli bool

	// watchers 管理 per-session watcher 取消函数，EndSession 时取消以避免
	// runner 补发的 error 事件把 status 覆盖为 error。
	mu       sync.Mutex
	watchers map[string]context.CancelFunc // sessionId → watcher cancel
	// runOpts 缓存 StartSession 入参，供重启型续接（codex/opencode/pty）复用。
	runOpts map[string]StartSessionOptions
}

// NewSessionService 创建 SessionService。
func NewSessionService(s *store.SessionStore, bus *eventbus.Bus, r *runner.Runner, mockCli bool) *SessionService {
	return &SessionService{
		store:    s,
		bus:      bus,
		runner:   r,
		mockCli:  mockCli,
		watchers: make(map[string]context.CancelFunc),
		runOpts:  make(map[string]StartSessionOptions),
	}
}

// ---- REST 端点（§4.14：REST 创建 status=idle）----

// Create 创建会话（REST POST /api/sessions）。status=idle，不启动 runner。
func (s *SessionService) Create(ctx context.Context, p types.CreateSessionParams) (*types.SessionDTO, error) {
	if p.Cli == "" {
		return nil, errors.New("cli is required")
	}
	sess := &store.Session{
		ID:        newID(),
		TaskID:    p.TaskID,
		Cli:       p.Cli,
		Model:     p.Model,
		Status:    types.SessionStatusIdle,
		CreatedAt: nowUTC(),
	}
	if err := s.store.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sessionToDTO(sess), nil
}

// Get 按 ID 查询会话。
func (s *SessionService) Get(ctx context.Context, id string) (*types.SessionDTO, error) {
	sess, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return sessionToDTO(sess), nil
}

// ListByTask 返回某任务下全部会话（createdAt desc）。
func (s *SessionService) ListByTask(ctx context.Context, taskID string) ([]*types.SessionDTO, error) {
	list, err := s.store.ListByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*types.SessionDTO, 0, len(list))
	for _, sess := range list {
		out = append(out, sessionToDTO(sess))
	}
	return out, nil
}

// Delete 删除会话（级联删除 events/file_snapshots）。
// 若会话仍在运行，先 Kill 进程。
func (s *SessionService) Delete(ctx context.Context, id string) error {
	// 停止 watcher + runner（幂等）
	s.stopWatcher(id)
	s.runner.Kill(id)
	if err := s.store.Delete(ctx, id); err != nil {
		return mapErr(err)
	}
	return nil
}

// GetEvents 返回某会话 eventId > since 的全部事件信封（REST GET /api/sessions/:id/events）。
func (s *SessionService) GetEvents(ctx context.Context, sessionID string, since int64) ([]*eventbus.Envelope, error) {
	return s.bus.GetEnvelopesSince(ctx, sessionID, since)
}

// ---- WS 生命周期（§2.8.1 / §2.4.2）----

// StartSessionOptions 是 WS session.start 的入参（§2.4.2）。
type StartSessionOptions struct {
	TaskID             *string
	ServerID           string // runReal 必填，runMock 忽略
	Cwd                string
	Cli                types.CliKind
	Model              string
	Prompt             string
	ResumeCliSessionID string // 续接时非空
	AllowedTools       []string
}

// StartSession 创建会话(status=running)并异步启动 runner（WS session.start 主流程）。
//
// sessionID 非 nil 时用作预设 ID（WS handler 先订阅再启动，保证不漏 session.init）；
// 为 nil 时后端生成新 cuid。
//
// 流程：
//  1. 建 Session(status=running)
//  2. 订阅事件总线（先于 runner 启动，避免漏 session.init）
//  3. 启动 watcher goroutine：监听 session.init → 写回 cliSessionId；session.end → 更新 status
//  4. 启动 runner（mockCli → RunMock，否则 RunReal）
func (s *SessionService) StartSession(ctx context.Context, sessionID *string, opts StartSessionOptions) (*types.SessionDTO, error) {
	if opts.Cli == "" {
		return nil, errors.New("cli is required")
	}
	if !s.mockCli && opts.ServerID == "" {
		return nil, errors.New("serverId is required (non-mock mode)")
	}

	id := newID()
	if sessionID != nil && *sessionID != "" {
		id = *sessionID
	}

	sess := &store.Session{
		ID:        id,
		TaskID:    opts.TaskID,
		Cli:       opts.Cli,
		Model:     opts.Model,
		Status:    types.SessionStatusRunning,
		CreatedAt: nowUTC(),
	}
	if err := s.store.Create(ctx, sess); err != nil {
		return nil, err
	}

	// 缓存启动参数，供重启型续接复用
	s.mu.Lock()
	s.runOpts[sess.ID] = opts
	s.mu.Unlock()

	// 订阅事件总线（先于 runner，确保不漏 session.init）
	ch, subCancel := s.bus.Subscribe(sess.ID)

	// watcher context：EndSession 时取消以跳过 runner 补发的 error 事件
	watchCtx, watchCancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.watchers[sess.ID] = watchCancel
	s.mu.Unlock()

	go s.watchSession(watchCtx, sess.ID, ch, subCancel)

	// 启动 runner
	runOpts := runner.RunOptions{
		SessionID:          sess.ID,
		ServerID:           opts.ServerID,
		Cwd:                opts.Cwd,
		Cli:                opts.Cli,
		Model:              opts.Model,
		Prompt:             opts.Prompt,
		ResumeCliSessionID: opts.ResumeCliSessionID,
		AllowedTools:       opts.AllowedTools,
	}

	var runErr error
	if s.mockCli {
		runErr = s.runner.RunMock(ctx, runOpts)
	} else {
		runErr = s.runner.RunReal(ctx, runOpts)
	}
	if runErr != nil {
		// runner 启动失败：清理 watcher + 更新 status=error
		s.stopWatcher(sess.ID)
		subCancel()
		endedAt := nowUTC()
		_ = s.store.UpdateStatus(ctx, sess.ID, types.SessionStatusError, &endedAt)
		slog.Error("start runner failed", "session", sess.ID, "error", runErr)
		return nil, runErr
	}

	return sessionToDTO(sess), nil
}

// SendInput 发送多轮输入（WS session.send）。
//
// stdin 型（claude-code/trae）：进程长驻，直接写 stdin。
// 重启型（codex/opencode/pty）：进程已结束，用缓存的启动参数 + DB 的 cliSessionId 重启续接。
func (s *SessionService) SendInput(ctx context.Context, sessionID, prompt string) error {
	// stdin 型：进程仍在运行
	if s.runner.IsRunning(sessionID) {
		if err := s.runner.SendInput(sessionID, prompt); err == nil {
			return nil
		}
		// SendInput 失败（非 stdin 型适配器）→ 走重启续接
	}

	// 重启型续接：查 session 获取 cliSessionId，复用缓存的启动参数
	sess, err := s.store.GetByID(ctx, sessionID)
	if err != nil {
		return mapErr(err)
	}

	s.mu.Lock()
	opts, ok := s.runOpts[sessionID]
	s.mu.Unlock()
	if !ok {
		return errors.New("session start options not found (cannot restart continuation)")
	}

	// 校验有 cliSessionId（pty 例外：§4.11 放行）
	isPty := opts.Cli == types.CliPty
	if !isPty && sess.CliSessionID == "" {
		return errors.New("session has no cliSessionId (cannot resume)")
	}

	// 更新 status=running
	if err := s.store.UpdateStatus(ctx, sessionID, types.SessionStatusRunning, nil); err != nil {
		return mapErr(err)
	}

	// 重新订阅 + 启动 watcher（旧 watcher 已随 session.end 退出）
	ch, subCancel := s.bus.Subscribe(sessionID)
	watchCtx, watchCancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.watchers[sessionID] = watchCancel
	s.mu.Unlock()
	go s.watchSession(watchCtx, sessionID, ch, subCancel)

	// 用 DB 的 cliSessionId 作为 resumeCliSessionId 重启
	runOpts := runner.RunOptions{
		SessionID:          sessionID,
		ServerID:           opts.ServerID,
		Cwd:                opts.Cwd,
		Cli:                opts.Cli,
		Model:              opts.Model,
		Prompt:             prompt,
		ResumeCliSessionID: sess.CliSessionID,
		AllowedTools:       opts.AllowedTools,
	}

	if s.mockCli {
		return s.runner.RunMock(ctx, runOpts)
	}
	return s.runner.RunReal(ctx, runOpts)
}

// Interrupt 中断当前回合（WS session.interrupt）。仅 runReal 模式有效。
func (s *SessionService) Interrupt(sessionID string) error {
	return s.runner.Interrupt(sessionID)
}

// EndSession 结束会话（WS session.end / WS close）。
// 取消 watcher → status=ended → runner.Kill。
func (s *SessionService) EndSession(ctx context.Context, sessionID string) error {
	// 先取消 watcher，避免 runner Kill 后补发 error 把 status 覆盖为 error
	s.stopWatcher(sessionID)

	endedAt := nowUTC()
	if err := s.store.UpdateStatus(ctx, sessionID, types.SessionStatusEnded, &endedAt); err != nil {
		// 会话不存在则忽略（幂等）
		if !errors.Is(err, store.ErrNotFound) {
			return mapErr(err)
		}
	}

	s.runner.Kill(sessionID)
	return nil
}

// Subscribe 订阅某会话的实时事件流（WS handler 用）。
// 返回 channel 和取消函数；WS 连接关闭时必须调用 cancel。
func (s *SessionService) Subscribe(sessionID string) (<-chan *eventbus.Envelope, func()) {
	return s.bus.Subscribe(sessionID)
}

// ---- 内部 ----

// watchSession 监听事件总线，处理 session.init（写回 cliSessionId）与 session.end（更新 status）。
//
// watchCtx 被 EndSession 取消时，watcher 直接退出（不更新 status，由 EndSession 负责）。
// 自然收到 session.end 时，根据是否见过 fatal error 决定 status=ended 或 error。
func (s *SessionService) watchSession(ctx context.Context, sessionID string, ch <-chan *eventbus.Envelope, subCancel func()) {
	defer subCancel()

	sawError := false
	for {
		select {
		case <-ctx.Done():
			// EndSession 取消：不更新 status（EndSession 已处理）
			s.removeWatcher(sessionID)
			return
		case env, ok := <-ch:
			if !ok {
				// 订阅被取消（如 bus 清理）：自然结束
				s.finalizeStatus(sessionID, sawError)
				return
			}
			switch ev := env.Event.(type) {
			case types.SessionInitEvent:
				if ev.CliSessionId != "" {
					if err := s.store.UpdateCliSessionID(context.Background(), sessionID, ev.CliSessionId); err != nil {
						slog.Warn("write cliSessionId failed", "session", sessionID, "error", err)
					}
				}
			case types.ErrorEvent:
				if ev.Recoverable != nil && !*ev.Recoverable {
					sawError = true
				}
			case types.SessionEndEvent:
				s.finalizeStatus(sessionID, sawError)
				return
			}
		}
	}
}

// finalizeStatus 在会话自然结束时更新 DB status。
func (s *SessionService) finalizeStatus(sessionID string, sawError bool) {
	endedAt := nowUTC()
	status := types.SessionStatusEnded
	if sawError {
		status = types.SessionStatusError
	}
	if err := s.store.UpdateStatus(context.Background(), sessionID, status, &endedAt); err != nil {
		slog.Warn("finalize session status failed", "session", sessionID, "status", status, "error", err)
	}
	s.removeWatcher(sessionID)
}

// stopWatcher 取消某会话的 watcher context（EndSession/Delete 用）。
func (s *SessionService) stopWatcher(sessionID string) {
	s.mu.Lock()
	cancel, ok := s.watchers[sessionID]
	if ok {
		delete(s.watchers, sessionID)
	}
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// removeWatcher 从 map 中移除 watcher（watcher 自身退出时用）。
func (s *SessionService) removeWatcher(sessionID string) {
	s.mu.Lock()
	delete(s.watchers, sessionID)
	s.mu.Unlock()
}

// sessionToDTO 把 store 实体转换为对外 DTO。
func sessionToDTO(sess *store.Session) *types.SessionDTO {
	return &types.SessionDTO{
		ID:           sess.ID,
		TaskID:       sess.TaskID,
		CliSessionID: sess.CliSessionID,
		Cli:          sess.Cli,
		Model:        sess.Model,
		Status:       string(sess.Status),
		CreatedAt:    sess.CreatedAt,
		EndedAt:      sess.EndedAt,
	}
}
