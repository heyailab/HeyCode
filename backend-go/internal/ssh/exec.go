package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

_ssh "golang.org/x/crypto/ssh"
)

// safeShellCharRe 匹配无需引号的安全字符（见 SPEC-GO-REWRITE.md §4.6）。
// 空串 → ''；纯安全字符 → 原样；其它 → 单引号包裹并转义内部单引号。
var safeShellCharRe = regexp.MustCompile(`^[A-Za-z0-9@%+=:,./_-]+$`)

// ShellQuote 按 POSIX shell 规则给字符串加引号。
//
//	""        → ''
//	abc-123   → abc-123
//	a b       → 'a b'
//	a'b       → 'a'\''b'
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if safeShellCharRe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ExecOptions 是 Exec 的可选参数。
type ExecOptions struct {
	// Cwd 工作目录，非空时拼接 `cd <quote(cwd)> && <command>` 前缀。
	Cwd string
	// TimeoutMs 超时毫秒数，<=0 表示不超时。
	TimeoutMs int
}

// ExecResult 是 Exec 的返回。
//
// ExitCode: 进程退出码；信号终止或 null → 0。
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Exec 在远端执行命令并收集 stdout/stderr/exitCode。
//
// 命令拼接：cwd 非空时 `cd <ShellQuote(cwd)> && <command>`，否则直接 <command>。
// timeoutMs <= 0 时不限时；超时后取消 session，返回 ctx.Err()。
func Exec(ctx context.Context, client *_ssh.Client, command string, opts ExecOptions) (*ExecResult, error) {
	fullCmd := command
	if opts.Cwd != "" {
		fullCmd = "cd " + ShellQuote(opts.Cwd) + " && " + command
	}

	// 应用超时
	execCtx := ctx
	var cancel context.CancelFunc
	if opts.TimeoutMs > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(opts.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new ssh session: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	// 用 execCtx 启动，超时会被 Session.Close 触发（在 execCtx done 后）
	done := make(chan error, 1)
	go func() {
		done <- sess.Run(fullCmd)
	}()

	select {
	case err := <-done:
		// ExitMissingError（信号终止无 exit code）视为 exitCode=0
		exitCode := 0
		if err != nil {
			var ee *_ssh.ExitError
			if errors.As(err, &ee) {
				exitCode = ee.ExitStatus()
			} else {
				// 非退出码错误（如网络断开）
				return nil, fmt.Errorf("ssh run: %w", err)
			}
		}
		return &ExecResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}, nil
	case <-execCtx.Done():
		// 超时：关闭 session 让 Run 返回
		_ = sess.Close()
		<-done // 等 goroutine 退出，避免泄漏
		return nil, execCtx.Err()
	}
}

// SpawnOptions 是 SpawnStream 的可选参数。
type SpawnOptions struct {
	// Cwd 工作目录，非空时拼接 cd 前缀（同 Exec）。
	Cwd string
	// Pty true 时调 session.RequestPty（pty 适配器）。
	Pty bool
	// Env 子进程环境变量（KEY=VALUE）。
	Env map[string]string
}

// Stream 是 spawnStream 返回的进程流句柄。
//
// Stdin: 写入用户输入（多轮续接）
// Stdout/Stderr: 读取进程输出（按行解析）
// Wait: 等待进程结束，返回 ExitError 或 nil
// Close: 释放 session（关闭 stdin/stdout pipe）
type Stream struct {
	Session *_ssh.Session
	Stdin   io.WriteCloser
	Stdout  io.Reader
	Stderr  io.Reader
}

// Wait 等待进程结束，返回退出码（信号终止或正常退出均映射为 int）。
// 网络错误时返回原始 error。
func (s *Stream) Wait() (int, error) {
	err := s.Session.Wait()
	if err == nil {
		return 0, nil
	}
	var ee *_ssh.ExitError
	if errors.As(err, &ee) {
		return ee.ExitStatus(), nil
	}
	return 0, err
}

// Close 释放 session。
func (s *Stream) Close() error {
	return s.Session.Close()
}

// SpawnStream 启动长驻进程，返回流式 stdin/stdout/stderr。
//
// command 是可执行文件名（如 "claude"），args 是参数列表。
// 拼接最终命令时 cwd 仍用 cd 前缀；args 由调用方自行 ShellQuote（适配器职责）。
func SpawnStream(ctx context.Context, client *_ssh.Client, command string, args []string, opts SpawnOptions) (*Stream, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new ssh session: %w", err)
	}

	// 设置环境变量
	if len(opts.Env) > 0 {
		for k, v := range opts.Env {
			// ssh.Session.Setenv 在多数 sshd 上要求配置 AcceptEnv，未必生效；
			// 适配器通常通过 command line 或 stdin 传 env，这里尽力而为。
			_ = sess.Setenv(k, v)
		}
	}

	// 请求 pty
	if opts.Pty {
		modes := _ssh.TerminalModes{
			_ssh.ECHO:          1,
			_ssh.TTY_OP_ISPEED: 14400,
			_ssh.TTY_OP_OSPEED: 14400,
		}
		if err := sess.RequestPty("xterm-256color", 80, 200, modes); err != nil {
			_ = sess.Close()
			return nil, fmt.Errorf("request pty: %w", err)
		}
	}

	// 建立 stdin pipe
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	// stdout/stderr 共享一个 reader（合并输出便于适配器按行解析）
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// 拼接命令
	fullCmd := strings.Join(append([]string{command}, args...), " ")
	if opts.Cwd != "" {
		fullCmd = "cd " + ShellQuote(opts.Cwd) + " && " + fullCmd
	}

	// Start 启动进程
	if err := sess.Start(fullCmd); err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("ssh start: %w", err)
	}

	// ctx 取消时关闭 session（让 Wait 返回）
	go func() {
		<-ctx.Done()
		_ = sess.Close()
	}()

	return &Stream{
		Session: sess,
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
	}, nil
}
