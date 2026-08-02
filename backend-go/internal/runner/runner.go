// Package runner 实现 process-runner：拉起 CLI 进程、按行解析输出、发布事件。
//
// 协议见 SPEC-GO-REWRITE.md §2.8.1 / §4.1。
// 核心职责：
//   - RunReal：通过 SSH 连接远端，spawnStream 拉起 CLI 进程，按行读取 → adapter.ParseLine → eventBus.Publish
//   - RunMock：不连 SSH，用 adapter.GenerateTimeline 产出模拟行序列，逐行喂给 ParseLine
//   - SendInput：向长驻进程 stdin 写入多轮输入（claude-code/trae）
//   - Interrupt：写 \x03 中断当前回合
//   - Kill：杀进程 + 释放资源
//
// per-session 运行状态（stream/ctx/cancel）由 Runner 管理，
// service/session.go 通过 Run/RunMock 启动，通过 SendInput/Interrupt/EndSession 控制。
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/heycode/backend-go/internal/adapter"
	"github.com/heycode/backend-go/internal/eventbus"
	"github.com/heycode/backend-go/internal/ssh"
	"github.com/heycode/backend-go/internal/types"
	_ssh "golang.org/x/crypto/ssh"
)

// Runner 管理活跃 CLI 进程，per-session。
//
// 线程安全：内部 mutex 保护 sessions map；
// 每个 sessionHandle 自身的并发控制由调用方（SessionService）保证。
type Runner struct {
	sshPool *ssh.Pool
	bus     *eventbus.Bus

	mu       sync.Mutex
	sessions map[string]*sessionHandle
}

// sessionHandle 是单个会话的运行时状态。
//
// stream 非 nil 表示 runReal 模式（SSH 长驻进程）；
// stream nil + mockTimeline 非空表示 runMock 模式。
type sessionHandle struct {
	mu       sync.Mutex // 串行化 SendInput/Interrupt/Kill
	sessionID string
	adapter  adapter.Adapter
	ctx      context.Context
	cancel   context.CancelFunc
	stream   *ssh.Stream // runReal 模式非空
	mockDone chan struct{} // runMock 模式：标记 timeline 播放完成
	// runReal 模式：stdout/stderr 合并读取后按行解析；进程结束由 Wait() 监听
	waitDone chan struct{} // 进程/timeline 结束信号
}

// RunOptions 是 Run / RunMock 的入参。
type RunOptions struct {
	SessionID         string
	ServerID          string // runReal 必填，runMock 忽略
	Cwd               string
	Cli               types.CliKind
	Model             string
	Prompt            string
	ResumeCliSessionID string // 续接时非空
	AllowedTools      []string
}

// New 创建 Runner。
func New(sshPool *ssh.Pool, bus *eventbus.Bus) *Runner {
	return &Runner{
		sshPool:  sshPool,
		bus:      bus,
		sessions: make(map[string]*sessionHandle),
	}
}

// RunReal 通过 SSH 拉起真实 CLI 进程（§2.8.1 runPrompt → runReal）。
//
// 流程：
//  1. adapter.Get(cli) 获取适配器
//  2. sshPool.Acquire 获取连接
//  3. adapter.BuildStartCommand 构造命令
//  4. ssh.SpawnStream 拉起进程（pty 适配器 opts.Pty=true）
//  5. 写入首条 prompt（adapter.BuildUserInput）
//  6. 启动 reader goroutine：按行读取 stdout/stderr → adapter.ParseLine → 逐事件 bus.Publish
//  7. 启动 waiter goroutine：等进程结束，未收到 session.end 则补发 error + session.end
//
// 本方法立即返回（不阻塞），进程在后台运行。调用方通过 EndSession/Kill 控制。
func (r *Runner) RunReal(ctx context.Context, opts RunOptions) error {
	a, err := adapter.Get(opts.Cli)
	if err != nil {
		return fmt.Errorf("get adapter: %w", err)
	}

	// 获取 SSH 连接
	client, err := r.sshPool.Acquire(ctx, opts.ServerID)
	if err != nil {
		return fmt.Errorf("ssh acquire: %w", err)
	}

	// 构造启动命令
	cmd := a.BuildStartCommand(adapter.BuildCommandOpts{
		Cwd:                opts.Cwd,
		Prompt:             opts.Prompt,
		Model:              opts.Model,
		ResumeCliSessionId: opts.ResumeCliSessionID,
		AllowedTools:       opts.AllowedTools,
	})

	// pty 适配器需要 RequestPty
	spawnOpts := ssh.SpawnOptions{Cwd: opts.Cwd, Env: cmd.Env}
	if opts.Cli == types.CliPty {
		spawnOpts.Pty = true
	}

	stream, err := ssh.SpawnStream(ctx, client, cmd.Command, cmd.Args, spawnOpts)
	if err != nil {
		return fmt.Errorf("spawn stream: %w", err)
	}

	// 写入首条 prompt（stdin 型适配器）
	if userInput := a.BuildUserInput(opts.Prompt); userInput != "" {
		if _, err := stream.Stdin.Write([]byte(userInput)); err != nil {
			slog.Warn("write initial prompt failed", "session", opts.SessionID, "error", err)
		}
	}

	// 创建 handle
	runCtx, cancel := context.WithCancel(ctx)
	h := &sessionHandle{
		sessionID: opts.SessionID,
		adapter:   a,
		ctx:       runCtx,
		cancel:    cancel,
		stream:    stream,
		waitDone:  make(chan struct{}),
	}

	r.mu.Lock()
	r.sessions[opts.SessionID] = h
	r.mu.Unlock()

	// reader goroutine：合并 stdout+stderr，按行解析
	go r.readLoop(h, client)

	// waiter goroutine：等进程结束
	go r.waitLoop(h, opts.SessionID)

	return nil
}

// RunMock 用 MockAdapter 的 GenerateTimeline 产出模拟事件流（§4.12）。
//
// 不连 SSH，用 25ms delay 逐行喂给 ParseLine，验证事件流端到端。
// 用于 MOCK_CLI=1 开发模式 + 单元测试。
func (r *Runner) RunMock(ctx context.Context, opts RunOptions) error {
	a, err := adapter.Get(opts.Cli)
	if err != nil {
		return fmt.Errorf("get adapter: %w", err)
	}
	gen, ok := a.(adapter.TimelineGenerator)
	if !ok {
		return fmt.Errorf("adapter %s does not support mock (no TimelineGenerator)", opts.Cli)
	}

	runCtx, cancel := context.WithCancel(ctx)
	h := &sessionHandle{
		sessionID: opts.SessionID,
		adapter:   a,
		ctx:       runCtx,
		cancel:    cancel,
		mockDone:  make(chan struct{}),
		waitDone:  make(chan struct{}),
	}

	r.mu.Lock()
	r.sessions[opts.SessionID] = h
	r.mu.Unlock()

	go r.mockLoop(h, gen, opts)
	return nil
}

// mockLoop 逐行喂 timeline 给 ParseLine，25ms delay。
func (r *Runner) mockLoop(h *sessionHandle, gen adapter.TimelineGenerator, opts RunOptions) {
	defer close(h.waitDone)

	timeline := gen.GenerateTimeline(opts.Prompt)
	parseCtx := adapter.NewParseContext(opts.SessionID, opts.Cwd, opts.Cli, opts.Model)

	for _, line := range timeline {
		select {
		case <-h.ctx.Done():
			return
		case <-time.After(25 * time.Millisecond):
		}
		ts := time.Now()
		events := h.adapter.ParseLine(line, parseCtx, ts.UnixMilli())
		for _, ev := range events {
			if _, err := r.bus.Publish(h.ctx, opts.SessionID, ev, ts); err != nil {
				slog.Warn("mock publish failed", "session", opts.SessionID, "error", err)
			}
		}
	}
}

// SendInput 向长驻进程写入多轮输入（claude-code/trae stdin 型）。
// 不支持 stdin 多轮的适配器（codex/opencode/pty）返回 error，调用方应改用重启续接。
func (r *Runner) SendInput(sessionID, prompt string) error {
	h := r.getHandle(sessionID)
	if h == nil {
		return fmt.Errorf("session %s not found or not running", sessionID)
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.stream == nil {
		return fmt.Errorf("session %s is not in runReal mode (mock or ended)", sessionID)
	}
	input := h.adapter.BuildUserInput(prompt)
	if input == "" {
		return fmt.Errorf("adapter %s does not support stdin multi-turn", h.adapter.Kind())
	}
	if _, err := h.stream.Stdin.Write([]byte(input)); err != nil {
		return fmt.Errorf("write stdin: %w", err)
	}
	return nil
}

// Interrupt 写 \x03 中断当前回合（§2.4.2 session.interrupt）。
// 仅 runReal 模式有效；mock 模式无进程可中断。
func (r *Runner) Interrupt(sessionID string) error {
	h := r.getHandle(sessionID)
	if h == nil {
		return fmt.Errorf("session %s not found or not running", sessionID)
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.stream == nil {
		return fmt.Errorf("session %s is not in runReal mode", sessionID)
	}
	_, err := h.stream.Stdin.Write([]byte{0x03})
	return err
}

// Kill 杀进程 + 释放资源（§2.4.2 session.end）。
// 幂等：多次调用安全。mock 模式 cancel context 即可。
func (r *Runner) Kill(sessionID string) {
	r.mu.Lock()
	h, ok := r.sessions[sessionID]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.sessions, sessionID)
	r.mu.Unlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	h.cancel() // 取消 context（reader/waiter/mockLoop 会退出）

	if h.stream != nil {
		_ = h.stream.Close()
	}
}

// IsRunning 判断会话是否在 Runner 中（活跃进程）。
func (r *Runner) IsRunning(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.sessions[sessionID]
	return ok
}

// WaitDone 返回某会话的结束信号 channel（进程/timeline 结束后关闭）。
// 用于 service 层等待会话自然结束。会话不存在或已结束返回 nil。
func (r *Runner) WaitDone(sessionID string) <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.sessions[sessionID]
	if !ok {
		return nil
	}
	return h.waitDone
}

// ---- 内部 ----

func (r *Runner) getHandle(sessionID string) *sessionHandle {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[sessionID]
}

// readLoop 按行读取进程输出，逐行 ParseLine → Publish。
//
// stdout 和 stderr 合并读取（SpawnStream 已分别提供 pipe），
// 这里用两个 goroutine 分别扫描，事件统一 Publish（per-session 锁保证顺序）。
func (r *Runner) readLoop(h *sessionHandle, client *_ssh.Client) {
	parseCtx := adapter.NewParseContext(h.sessionID, "", h.adapter.Kind(), "")

	// stderr 通常无结构化输出，作为日志记录
	go scanLines(h.stream.Stderr, func(line string) {
		slog.Debug("cli stderr", "session", h.sessionID, "line", line)
	})

	// stdout 是主要事件源
	scanLines(h.stream.Stdout, func(line string) {
		ts := time.Now()
		events := h.adapter.ParseLine(line, parseCtx, ts.UnixMilli())
		for _, ev := range events {
			if _, err := r.bus.Publish(h.ctx, h.sessionID, ev, ts); err != nil {
				slog.Warn("publish failed", "session", h.sessionID, "error", err)
			}
		}
	})

	_ = client // client 由 pool 管理，不在此关闭
}

// waitLoop 等进程结束，补发 error + session.end（§2.8.1）。
//
// 若进程异常退出且适配器未发 session.end（如进程崩溃），
// 后端补发 error(recoverable=false) + session.end，保证客户端能感知会话结束。
func (r *Runner) waitLoop(h *sessionHandle, sessionID string) {
	defer close(h.waitDone)

	exitCode, err := h.stream.Wait()
	ts := time.Now()

	if err != nil {
		// 进程异常退出（网络断开/信号终止）
		falseVal := false
		errEv := types.NewError(ts.UnixMilli(), fmt.Sprintf("CLI 进程异常退出: %v", err), &falseVal, string(h.adapter.Kind()))
		if _, perr := r.bus.Publish(context.Background(), sessionID, errEv, ts); perr != nil {
			slog.Warn("publish error event on exit failed", "session", sessionID, "error", perr)
		}
	} else if exitCode != 0 {
		// 非零退出（CLI 自身报错）
		falseVal := false
		errEv := types.NewError(ts.UnixMilli(), fmt.Sprintf("CLI 进程退出码 %d", exitCode), &falseVal, string(h.adapter.Kind()))
		if _, perr := r.bus.Publish(context.Background(), sessionID, errEv, ts); perr != nil {
			slog.Warn("publish error event on exit failed", "session", sessionID, "error", perr)
		}
	}

	// 补发 session.end（若适配器未发）
	endEv := types.NewSessionEnd(ts.UnixMilli(), nil)
	if _, perr := r.bus.Publish(context.Background(), sessionID, endEv, ts); perr != nil {
		slog.Warn("publish session.end on exit failed", "session", sessionID, "error", perr)
	}

	// 从 Runner 注销
	r.mu.Lock()
	delete(r.sessions, sessionID)
	r.mu.Unlock()
}

// scanLines 按行扫描 reader，每行调用 handler（含尾部无换行的最后一行）。
func scanLines(reader io.Reader, handler func(string)) {
	scanner := bufio.NewScanner(reader)
	// CLI 输出可能含长行（如文件内容），提高缓冲上限
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		handler(scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		slog.Debug("scanner error", "error", err)
	}
}
