package adapter

import (
	"strings"
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// ==== Kind / BuildStartCommand / BuildUserInput ====

func TestPtyKind(t *testing.T) {
	if (&PtyAdapter{}).Kind() != types.CliPty {
		t.Errorf("Kind() != pty")
	}
}

func TestPtyBuildStartCommandBasic(t *testing.T) {
	cmd := (&PtyAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:    "/c",
		Prompt: "hello",
	})
	if cmd.Command != "lingma" {
		t.Errorf("Command = %q, want lingma", cmd.Command)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--cwd /c") {
		t.Errorf("missing --cwd: %v", cmd.Args)
	}
	if !strings.Contains(joined, "hello") {
		t.Errorf("missing prompt: %v", cmd.Args)
	}
}

func TestPtyBuildStartCommandWithModel(t *testing.T) {
	cmd := (&PtyAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:    "/c",
		Prompt: "p",
		Model:  "qwen-max",
	})
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--model qwen-max") {
		t.Errorf("missing --model: %v", cmd.Args)
	}
}

func TestPtyBuildUserInput(t *testing.T) {
	if (&PtyAdapter{}).BuildUserInput("p") != "" {
		t.Errorf("pty BuildUserInput should return empty (no stdin multi-turn)")
	}
}

// ==== stripAnsi ====

func TestStripAnsi(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain text", "plain text"},
		{"\x1b[31mred text\x1b[0m", "red text"},            // 颜色码
		{"\x1b[1;32mgreen bold\x1b[0m", "green bold"},       // 多参数
		{"\x1b[2J\x1b[H", ""},                               // 清屏 + 光标归位
		{"mix \x1b[33myellow\x1b[0m and plain", "mix yellow and plain"},
		{"中文无 ANSI", "中文无 ANSI"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripAnsi(c.in); got != c.want {
			t.Errorf("stripAnsi(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ==== ParseLine ====

// TestPtyParseFirstLine 首行 → session.init + progress + command.exec
func TestPtyParseFirstLine(t *testing.T) {
	ctx := NewParseContext("sess-1", "/c", types.CliPty, "qwen-max")
	a := &PtyAdapter{}
	line := "some output line"
	evs := a.ParseLine(line, ctx, 100)
	// 期望：session.init + progress + command.exec
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(evs), evs)
	}
	si, ok := evs[0].(types.SessionInitEvent)
	if !ok {
		t.Fatalf("events[0] should be SessionInitEvent, got %T", evs[0])
	}
	if si.SessionId != "sess-1" || si.Cli != "pty" || si.Model != "qwen-max" {
		t.Errorf("SessionInitEvent = %+v", si)
	}
	if si.CliSessionId != "" {
		t.Errorf("CliSessionId should be empty for pty, got %q", si.CliSessionId)
	}
	p, ok := evs[1].(types.ProgressEvent)
	if !ok {
		t.Fatalf("events[1] should be ProgressEvent, got %T", evs[1])
	}
	if p.Message != "PTY 会话已启动" {
		t.Errorf("Progress message = %q", p.Message)
	}
	ce, ok := evs[2].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("events[2] should be CommandExecEvent, got %T", evs[2])
	}
	if ce.Stdout != "some output line" {
		t.Errorf("Stdout = %q", ce.Stdout)
	}
	if ce.Cwd != "/c" {
		t.Errorf("Cwd = %q", ce.Cwd)
	}
	if ce.ExitCode != nil {
		t.Errorf("ExitCode should be nil (in progress), got %v", *ce.ExitCode)
	}
	// 标记已发
	if !ctx.sessionInitSent {
		t.Errorf("sessionInitSent should be true after first line")
	}
}

// TestPtyParseSubsequentLine 后续行 → 只 command.exec
func TestPtyParseSubsequentLine(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliPty, "")
	ctx.sessionInitSent = true // 已发过
	a := &PtyAdapter{}
	evs := a.ParseLine("line 2", ctx, 200)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (command.exec only), got %d: %v", len(evs), evs)
	}
	ce, ok := evs[0].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("expected CommandExecEvent, got %T", evs[0])
	}
	if ce.Stdout != "line 2" {
		t.Errorf("Stdout = %q", ce.Stdout)
	}
}

// TestPtyParseAnsiStripped ANSI 序列应被剥离
func TestPtyParseAnsiStripped(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliPty, "")
	ctx.sessionInitSent = true
	a := &PtyAdapter{}
	evs := a.ParseLine("\x1b[32mgreen output\x1b[0m", ctx, 1)
	ce := evs[0].(types.CommandExecEvent)
	if ce.Stdout != "green output" {
		t.Errorf("Stdout should be ANSI-stripped, got %q", ce.Stdout)
	}
}

// TestPtyParseCRLF 行尾 \r\n 应被去除
func TestPtyParseCRLF(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliPty, "")
	ctx.sessionInitSent = true
	a := &PtyAdapter{}
	evs := a.ParseLine("text\r\n", ctx, 1)
	ce := evs[0].(types.CommandExecEvent)
	if ce.Stdout != "text" {
		t.Errorf("Stdout = %q, want text (CRLF stripped)", ce.Stdout)
	}
}

// TestPtyParseEndMarker __PTY_END__ → streaming.done + session.end
func TestPtyParseEndMarker(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliPty, "")
	ctx.sessionInitSent = true
	ctx.CurrentMessageId = "msg-1"
	a := &PtyAdapter{}
	evs := a.ParseLine("output before end __PTY_END__", ctx, 500)
	// 期望：command.exec(含 end marker 行) + streaming.done + session.end
	// 注意：end marker 行本身也会先发 command.exec（首行逻辑在 marker 检查之前）
	// 重新审视实现：首行逻辑在 marker 检查之前，但 marker 检查在 command.exec 之前 return
	// 实际实现顺序：首行→marker 检查→command.exec。marker 命中时 return，不发 command.exec
	// 所以期望：streaming.done + session.end
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (streaming.done + session.end), got %d: %v", len(evs), evs)
	}
	sd, ok := evs[0].(types.StreamingDoneEvent)
	if !ok {
		t.Fatalf("events[0] should be StreamingDoneEvent, got %T", evs[0])
	}
	if sd.MessageId != "msg-1" {
		t.Errorf("MessageId = %q", sd.MessageId)
	}
	if _, ok := evs[1].(types.SessionEndEvent); !ok {
		t.Errorf("events[1] should be SessionEndEvent, got %T", evs[1])
	}
}

// TestPtyParseEndMarkerFirstLine __PTY_END__ 出现在首行
func TestPtyParseEndMarkerFirstLine(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliPty, "")
	a := &PtyAdapter{}
	// 首行含 marker：先发 session.init + progress，再发 streaming.done + session.end
	evs := a.ParseLine("__PTY_END__", ctx, 1)
	if len(evs) != 4 {
		t.Fatalf("expected 4 events (session.init + progress + streaming.done + session.end), got %d: %v", len(evs), evs)
	}
	if _, ok := evs[0].(types.SessionInitEvent); !ok {
		t.Errorf("events[0] should be SessionInitEvent, got %T", evs[0])
	}
	if _, ok := evs[1].(types.ProgressEvent); !ok {
		t.Errorf("events[1] should be ProgressEvent, got %T", evs[1])
	}
	if _, ok := evs[2].(types.StreamingDoneEvent); !ok {
		t.Errorf("events[2] should be StreamingDoneEvent, got %T", evs[2])
	}
	if _, ok := evs[3].(types.SessionEndEvent); !ok {
		t.Errorf("events[3] should be SessionEndEvent, got %T", evs[3])
	}
}
