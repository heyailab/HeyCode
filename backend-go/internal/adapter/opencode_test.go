package adapter

import (
	"strings"
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// ==== Kind / BuildStartCommand / BuildUserInput ====

func TestOpencodeKind(t *testing.T) {
	if (&OpencodeAdapter{}).Kind() != types.CliOpencode {
		t.Errorf("Kind() != opencode")
	}
}

func TestOpencodeBuildStartCommandBasic(t *testing.T) {
	cmd := (&OpencodeAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:    "/c",
		Prompt: "do task",
	})
	if cmd.Command != "opencode" {
		t.Errorf("Command = %q, want opencode", cmd.Command)
	}
	joined := strings.Join(cmd.Args, " ")
	wantFragments := []string{
		"run --format json --dangerously-skip-permissions --cwd /c",
		"do task", // prompt 作参数
	}
	for _, w := range wantFragments {
		if !strings.Contains(joined, w) {
			t.Errorf("missing fragment %q in args: %v", w, cmd.Args)
		}
	}
}

func TestOpencodeBuildStartCommandWithModelAndContinue(t *testing.T) {
	cmd := (&OpencodeAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:                "/c",
		Prompt:             "p",
		Model:              "claude-sonnet-4",
		ResumeCliSessionId: "sess-xyz",
	})
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--model claude-sonnet-4") {
		t.Errorf("missing --model: %v", cmd.Args)
	}
	if !strings.Contains(joined, "--continue sess-xyz") {
		t.Errorf("missing --continue: %v", cmd.Args)
	}
}

func TestOpencodeBuildUserInput(t *testing.T) {
	if (&OpencodeAdapter{}).BuildUserInput("p") != "" {
		t.Errorf("opencode BuildUserInput should return empty (no stdin)")
	}
}

// ==== ParseLine ====

func TestOpencodeParseEmptyLine(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	if evs := (&OpencodeAdapter{}).ParseLine("", ctx, 1); evs != nil {
		t.Errorf("empty line should return nil")
	}
}

// TestOpencodeParseStepStartFirst 首次 step_start → session.init + progress
func TestOpencodeParseStepStartFirst(t *testing.T) {
	ctx := NewParseContext("sess-1", "/c", types.CliOpencode, "claude-sonnet-4")
	line := `{"type":"step_start"}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 100)
	// 期望：session.init + progress
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (session.init + progress), got %d: %v", len(evs), evs)
	}
	si, ok := evs[0].(types.SessionInitEvent)
	if !ok {
		t.Fatalf("events[0] should be SessionInitEvent, got %T", evs[0])
	}
	if si.SessionId != "sess-1" || si.Cli != "opencode" || si.Model != "claude-sonnet-4" {
		t.Errorf("SessionInitEvent = %+v", si)
	}
	if si.CliSessionId != "" {
		t.Errorf("CliSessionId should be empty for opencode, got %q", si.CliSessionId)
	}
	p, ok := evs[1].(types.ProgressEvent)
	if !ok {
		t.Fatalf("events[1] should be ProgressEvent, got %T", evs[1])
	}
	if p.Step == nil || *p.Step != 1 || p.Total != nil {
		t.Errorf("Progress = %+v, want step=1 total=nil", p)
	}
	// 验证已标记 sessionInitSent
	if !ctx.sessionInitSent {
		t.Errorf("sessionInitSent should be true after first step_start")
	}
	// 验证累积重置
	if ctx.CurrentMessageId == "" || ctx.CurrentRole != "assistant" {
		t.Errorf("accumulation should be reset: id=%q role=%q", ctx.CurrentMessageId, ctx.CurrentRole)
	}
}

// TestOpencodeParseStepStartSecond 第二次 step_start → 只 progress（无 session.init）
func TestOpencodeParseStepStartSecond(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	ctx.sessionInitSent = true // 已发过
	line := `{"type":"step_start"}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 200)
	// 期望：只有 progress
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (progress only), got %d: %v", len(evs), evs)
	}
	if _, ok := evs[0].(types.ProgressEvent); !ok {
		t.Errorf("events[0] should be ProgressEvent, got %T", evs[0])
	}
}

// TestOpencodeParseText text → streaming.delta + 累积
func TestOpencodeParseText(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	ctx.CurrentMessageId = "msg-1"
	line := `{"type":"text","text":"chunk-1"}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 100)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	sd, ok := evs[0].(types.StreamingDeltaEvent)
	if !ok {
		t.Fatalf("expected StreamingDeltaEvent, got %T", evs[0])
	}
	if sd.MessageId != "msg-1" || sd.TextDelta != "chunk-1" {
		t.Errorf("StreamingDeltaEvent = %+v", sd)
	}
	// 累积
	if len(ctx.CurrentBlocks) != 1 {
		t.Fatalf("CurrentBlocks should accumulate, got %v", ctx.CurrentBlocks)
	}
}

// TestOpencodeParseTextEmpty text 为空 → nil
func TestOpencodeParseTextEmpty(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	if evs := (&OpencodeAdapter{}).ParseLine(`{"type":"text","text":""}`, ctx, 1); evs != nil {
		t.Errorf("empty text should return nil, got %v", evs)
	}
}

// TestOpencodeParseReasoning reasoning → thinking
func TestOpencodeParseReasoning(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	line := `{"type":"reasoning","text":"thinking..."}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	th, ok := evs[0].(types.ThinkingEvent)
	if !ok {
		t.Fatalf("expected ThinkingEvent, got %T", evs[0])
	}
	if th.Text != "thinking..." {
		t.Errorf("Text = %q", th.Text)
	}
}

// TestOpencodeParseToolStartWrite write → tool.use + file.change(create) + 入队
func TestOpencodeParseToolStartWrite(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	line := `{"type":"tool_start","id":"oc-1","name":"write","input":{"path":"/tmp/new.go"}}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 100)
	// 期望：tool.use + file.change
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(evs), evs)
	}
	tu, ok := evs[0].(types.ToolUseEvent)
	if !ok {
		t.Fatalf("events[0] should be ToolUseEvent, got %T", evs[0])
	}
	if tu.ToolUseId != "oc-1" || tu.ToolName != "write" {
		t.Errorf("ToolUseEvent = %+v", tu)
	}
	fc, ok := evs[1].(types.FileChangeEvent)
	if !ok {
		t.Fatalf("events[1] should be FileChangeEvent, got %T", evs[1])
	}
	if fc.Change.Path != "/tmp/new.go" || fc.Change.Action != "create" {
		t.Errorf("FileChange = %+v", fc.Change)
	}
	// 入队
	if len(ctx.PendingToolUseIds) != 1 || ctx.PendingToolUseIds[0] != "oc-1" {
		t.Errorf("PendingToolUseIds = %v, want [oc-1]", ctx.PendingToolUseIds)
	}
}

// TestOpencodeParseToolStartEdit edit → action=edit
func TestOpencodeParseToolStartEdit(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	line := `{"type":"tool_start","id":"oc-e","name":"edit","input":{"file_path":"/tmp/e.go"}}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 1)
	fc := evs[1].(types.FileChangeEvent)
	if fc.Change.Action != "edit" {
		t.Errorf("edit action = %q, want edit", fc.Change.Action)
	}
	if fc.Change.Path != "/tmp/e.go" {
		t.Errorf("Path = %q (should accept file_path fallback)", fc.Change.Path)
	}
}

// TestOpencodeParseToolStartBash bash → tool.use + command.exec(started) + 入队
func TestOpencodeParseToolStartBash(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	line := `{"type":"tool_start","id":"oc-b","name":"bash","input":{"command":"pwd"}}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 100)
	// 期望：tool.use + command.exec(started)
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(evs), evs)
	}
	ce, ok := evs[1].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("events[1] should be CommandExecEvent, got %T", evs[1])
	}
	if ce.Command != "pwd" {
		t.Errorf("Command = %q", ce.Command)
	}
	if ce.ExitCode != nil {
		t.Errorf("ExitCode should be nil for started, got %v", *ce.ExitCode)
	}
	if ce.ToolUseId != "oc-b" {
		t.Errorf("ToolUseId = %q", ce.ToolUseId)
	}
}

// TestOpencodeParseToolStartOther 其他工具 → 只有 tool.use（无衍生）
func TestOpencodeParseToolStartOther(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	line := `{"type":"tool_start","id":"oc-r","name":"read","input":{"path":"/tmp/x"}}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (tool.use only), got %d", len(evs))
	}
}

// TestOpencodeParseToolFinishBash bash 完成 → tool.result + command.exec(completed)
func TestOpencodeParseToolFinishBash(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	// 先入队 bash 工具
	ctx.EnqueueToolUse("oc-b", ToolUseInfo{
		Name:  "bash",
		Input: []byte(`{"command":"pwd"}`),
	})
	line := `{"type":"tool_finish","output":"/home/user","exit_code":0}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 200)
	// 期望：tool.result + command.exec(completed)
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(evs), evs)
	}
	tr, ok := evs[0].(types.ToolResultEvent)
	if !ok {
		t.Fatalf("events[0] should be ToolResultEvent, got %T", evs[0])
	}
	if tr.ToolUseId != "oc-b" {
		t.Errorf("ToolUseId = %q (FIFO shift)", tr.ToolUseId)
	}
	if tr.IsError {
		t.Errorf("IsError should be false")
	}
	ce, ok := evs[1].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("events[1] should be CommandExecEvent, got %T", evs[1])
	}
	if ce.Command != "pwd" {
		t.Errorf("Command = %q", ce.Command)
	}
	if ce.ExitCode == nil || *ce.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", ce.ExitCode)
	}
	if ce.Stdout != "/home/user" {
		t.Errorf("Stdout = %q", ce.Stdout)
	}
	// 清理
	if _, ok := ctx.LookupToolUse("oc-b"); ok {
		t.Errorf("tool_use should be forgotten after tool_finish")
	}
}

// TestOpencodeParseToolFinishError error 字段 → isError + exit=1 + stderr
func TestOpencodeParseToolFinishError(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	ctx.EnqueueToolUse("oc-e", ToolUseInfo{
		Name:  "bash",
		Input: []byte(`{"command":"bad"}`),
	})
	line := `{"type":"tool_finish","error":"command failed"}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 1)
	tr := evs[0].(types.ToolResultEvent)
	if !tr.IsError {
		t.Errorf("IsError should be true when error field set")
	}
	ce := evs[1].(types.CommandExecEvent)
	if ce.ExitCode == nil || *ce.ExitCode != 1 {
		t.Errorf("ExitCode = %v, want 1", ce.ExitCode)
	}
	if ce.Stderr != "command failed" {
		t.Errorf("Stderr = %q", ce.Stderr)
	}
}

// TestOpencodeParseToolFinishEmptyQueue 空队列 → nil
func TestOpencodeParseToolFinishEmptyQueue(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	line := `{"type":"tool_finish","output":"x"}`
	if evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 1); evs != nil {
		t.Errorf("tool_finish with empty queue should return nil, got %v", evs)
	}
}

// TestOpencodeParseToolFinishNonBash 非 bash 工具完成 → 只有 tool.result
func TestOpencodeParseToolFinishNonBash(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	ctx.EnqueueToolUse("oc-w", ToolUseInfo{
		Name:  "write",
		Input: []byte(`{"path":"/tmp/x"}`),
	})
	line := `{"type":"tool_finish","output":"ok"}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (tool.result only for non-bash), got %d", len(evs))
	}
	if _, ok := evs[0].(types.ToolResultEvent); !ok {
		t.Errorf("events[0] should be ToolResultEvent, got %T", evs[0])
	}
}

// TestOpencodeParseStepFinish step_finish → message + streaming.done + session.end
func TestOpencodeParseStepFinish(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	ctx.CurrentMessageId = "msg-1"
	ctx.CurrentRole = "assistant"
	ctx.CurrentBlocks = []types.ContentBlock{types.TextBlock{Type: "text", Text: "done"}}

	line := `{"type":"step_finish"}`
	evs := (&OpencodeAdapter{}).ParseLine(line, ctx, 300)
	// 期望：message + streaming.done + session.end
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(evs), evs)
	}
	msg, ok := evs[0].(types.MessageEvent)
	if !ok {
		t.Fatalf("events[0] should be MessageEvent, got %T", evs[0])
	}
	if len(msg.Blocks) != 1 {
		t.Errorf("Message blocks = %d", len(msg.Blocks))
	}
	if _, ok := evs[1].(types.StreamingDoneEvent); !ok {
		t.Errorf("events[1] should be StreamingDoneEvent, got %T", evs[1])
	}
	se, ok := evs[2].(types.SessionEndEvent)
	if !ok {
		t.Fatalf("events[2] should be SessionEndEvent, got %T", evs[2])
	}
	if se.Stats != nil {
		t.Errorf("opencode step_finish should have nil stats, got %+v", se.Stats)
	}
	// 清空
	if ctx.CurrentMessageId != "" || len(ctx.CurrentBlocks) != 0 {
		t.Errorf("accumulation should be cleared after step_finish")
	}
}

// TestOpencodeParseFIFOOrder 验证 FIFO 顺序：多个 tool_start 后 tool_finish 按入队顺序匹配
func TestOpencodeParseFIFOOrder(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliOpencode, "")
	a := &OpencodeAdapter{}

	// 入队两个 bash
	a.ParseLine(`{"type":"tool_start","id":"t1","name":"bash","input":{"command":"cmd1"}}`, ctx, 100)
	a.ParseLine(`{"type":"tool_start","id":"t2","name":"bash","input":{"command":"cmd2"}}`, ctx, 200)

	// 第一个 finish 应匹配 t1
	evs := a.ParseLine(`{"type":"tool_finish","output":"out1","exit_code":0}`, ctx, 300)
	tr := evs[0].(types.ToolResultEvent)
	if tr.ToolUseId != "t1" {
		t.Errorf("first finish should match t1 (FIFO), got %q", tr.ToolUseId)
	}

	// 第二个 finish 应匹配 t2
	evs = a.ParseLine(`{"type":"tool_finish","output":"out2","exit_code":0}`, ctx, 400)
	tr = evs[0].(types.ToolResultEvent)
	if tr.ToolUseId != "t2" {
		t.Errorf("second finish should match t2 (FIFO), got %q", tr.ToolUseId)
	}
}

// ==== 辅助函数 ====

func TestRawToString(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`"hello"`, "hello"},
		{`"with spaces"`, "with spaces"},
		{``, ""}, // 空
	}
	for _, c := range cases {
		var raw []byte
		if c.raw != "" {
			raw = []byte(c.raw)
		}
		if got := rawToString(raw); got != c.want {
			t.Errorf("rawToString(%s) = %q, want %q", c.raw, got, c.want)
		}
	}
}
