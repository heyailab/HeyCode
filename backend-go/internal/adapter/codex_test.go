package adapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// ==== Kind / BuildStartCommand / BuildUserInput ====

func TestCodexKind(t *testing.T) {
	if (&CodexAdapter{}).Kind() != types.CliCodex {
		t.Errorf("Kind() != codex")
	}
}

func TestCodexBuildStartCommandBasic(t *testing.T) {
	cmd := (&CodexAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:    "/c",
		Prompt: "fix the bug",
	})
	if cmd.Command != "codex" {
		t.Errorf("Command = %q, want codex", cmd.Command)
	}
	joined := strings.Join(cmd.Args, " ")
	wantFragments := []string{
		"exec --json --full-auto --skip-git-repo-check --cd /c",
		"fix the bug", // prompt 作参数
	}
	for _, w := range wantFragments {
		if !strings.Contains(joined, w) {
			t.Errorf("missing fragment %q in args: %v", w, cmd.Args)
		}
	}
}

func TestCodexBuildStartCommandWithModel(t *testing.T) {
	cmd := (&CodexAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:    "/c",
		Prompt: "p",
		Model:  "gpt-5",
	})
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--model gpt-5") {
		t.Errorf("missing --model: %v", cmd.Args)
	}
}

func TestCodexBuildStartCommandWithResume(t *testing.T) {
	cmd := (&CodexAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:                "/c",
		Prompt:             "continue",
		ResumeCliSessionId: "thread-123",
	})
	joined := strings.Join(cmd.Args, " ")
	// 续接：resume <sid> "<prompt>"
	if !strings.Contains(joined, "resume thread-123 continue") {
		t.Errorf("missing resume clause: %v", cmd.Args)
	}
}

func TestCodexBuildUserInput(t *testing.T) {
	if (&CodexAdapter{}).BuildUserInput("p") != "" {
		t.Errorf("codex BuildUserInput should return empty (no stdin multi-turn)")
	}
}

// ==== ParseLine ====

func TestCodexParseEmptyLine(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	if evs := (&CodexAdapter{}).ParseLine("  ", ctx, 1); evs != nil {
		t.Errorf("empty line should return nil")
	}
}

func TestCodexParseNonJSON(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	if evs := (&CodexAdapter{}).ParseLine("garbage", ctx, 1); evs != nil {
		t.Errorf("non-JSON line should return nil, got %v", evs)
	}
}

func TestCodexParseUnknownType(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	if evs := (&CodexAdapter{}).ParseLine(`{"type":"unknown"}`, ctx, 1); evs != nil {
		t.Errorf("unknown type should return nil, got %v", evs)
	}
}

// TestCodexParseThreadStarted thread.started → session.init
func TestCodexParseThreadStarted(t *testing.T) {
	ctx := NewParseContext("sess-1", "/c", types.CliCodex, "gpt-5")
	line := `{"type":"thread.started","thread_id":"thread-abc"}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 100)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	si, ok := evs[0].(types.SessionInitEvent)
	if !ok {
		t.Fatalf("expected SessionInitEvent, got %T", evs[0])
	}
	if si.CliSessionId != "thread-abc" {
		t.Errorf("CliSessionId = %q, want thread-abc", si.CliSessionId)
	}
	if si.Cli != "codex" {
		t.Errorf("Cli = %q", si.Cli)
	}
	if si.Model != "gpt-5" {
		t.Errorf("Model = %q", si.Model)
	}
}

// TestCodexParseTurnStarted turn.started → progress + 重置累积
func TestCodexParseTurnStarted(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	// 预填一些累积，验证 turn.started 会重置
	ctx.CurrentBlocks = []types.ContentBlock{types.TextBlock{Type: "text", Text: "stale"}}
	ctx.CurrentMessageId = "old"

	line := `{"type":"turn.started"}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1000)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (progress), got %d", len(evs))
	}
	p, ok := evs[0].(types.ProgressEvent)
	if !ok {
		t.Fatalf("expected ProgressEvent, got %T", evs[0])
	}
	if p.Step == nil || *p.Step != 1 || p.Total == nil || *p.Total != 1 {
		t.Errorf("Progress = %+v, want step=1 total=1", p)
	}
	// 验证重置
	if len(ctx.CurrentBlocks) != 0 {
		t.Errorf("CurrentBlocks should be reset, got %v", ctx.CurrentBlocks)
	}
	if ctx.CurrentMessageId == "old" || ctx.CurrentMessageId == "" {
		t.Errorf("CurrentMessageId should be regenerated, got %q", ctx.CurrentMessageId)
	}
	if ctx.CurrentRole != "assistant" {
		t.Errorf("CurrentRole = %q, want assistant", ctx.CurrentRole)
	}
	if ctx.currentTurnStats == nil {
		t.Errorf("currentTurnStats should be initialized")
	}
}

// TestCodexParseAgentMessage item.completed(agent_message) → streaming.delta + 累积
func TestCodexParseAgentMessage(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	ctx.CurrentMessageId = "msg-1"
	line := `{"type":"item.completed","item":{"type":"agent_message","text":"hello world"}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 100)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (streaming.delta), got %d", len(evs))
	}
	sd, ok := evs[0].(types.StreamingDeltaEvent)
	if !ok {
		t.Fatalf("expected StreamingDeltaEvent, got %T", evs[0])
	}
	if sd.MessageId != "msg-1" {
		t.Errorf("MessageId = %q", sd.MessageId)
	}
	if sd.TextDelta != "hello world" {
		t.Errorf("TextDelta = %q", sd.TextDelta)
	}
	// 累积到 CurrentBlocks
	if len(ctx.CurrentBlocks) != 1 {
		t.Fatalf("CurrentBlocks should accumulate, got %v", ctx.CurrentBlocks)
	}
	tb := ctx.CurrentBlocks[0].(types.TextBlock)
	if tb.Text != "hello world" {
		t.Errorf("accumulated Text = %q", tb.Text)
	}
}

// TestCodexParseAgentMessageOldSchema 旧版 assistant_message 应归一为 agent_message
func TestCodexParseAgentMessageOldSchema(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	ctx.CurrentMessageId = "msg-1"
	line := `{"type":"item.completed","item":{"type":"assistant_message","text":"legacy"}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event for legacy assistant_message, got %d", len(evs))
	}
	if _, ok := evs[0].(types.StreamingDeltaEvent); !ok {
		t.Errorf("expected StreamingDeltaEvent, got %T", evs[0])
	}
}

// TestCodexParseReasoning item.completed(reasoning) → thinking
func TestCodexParseReasoning(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"reasoning","text":"let me think"}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	th, ok := evs[0].(types.ThinkingEvent)
	if !ok {
		t.Fatalf("expected ThinkingEvent, got %T", evs[0])
	}
	if th.Text != "let me think" {
		t.Errorf("Text = %q", th.Text)
	}
}

// TestCodexParseCommandExecutionCompleted 含 exit_code → tool.use + tool.result + command.exec
func TestCodexParseCommandExecutionCompleted(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"command_execution","id":"cmd-1","command":"ls","exit_code":0,"stdout":"file1\nfile2"}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 100)
	// 期望：tool.use + tool.result + command.exec
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(evs), evs)
	}
	tu, ok := evs[0].(types.ToolUseEvent)
	if !ok {
		t.Fatalf("events[0] should be ToolUseEvent, got %T", evs[0])
	}
	if tu.ToolUseId != "cmd-1" || tu.ToolName != "Bash" {
		t.Errorf("ToolUseEvent = %+v", tu)
	}
	// tool.result
	tr, ok := evs[1].(types.ToolResultEvent)
	if !ok {
		t.Fatalf("events[1] should be ToolResultEvent, got %T", evs[1])
	}
	if tr.ToolUseId != "cmd-1" || tr.IsError {
		t.Errorf("ToolResultEvent = %+v", tr)
	}
	// command.exec
	ce, ok := evs[2].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("events[2] should be CommandExecEvent, got %T", evs[2])
	}
	if ce.Command != "ls" {
		t.Errorf("Command = %q", ce.Command)
	}
	if ce.ExitCode == nil || *ce.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", ce.ExitCode)
	}
	if ce.Stdout != "file1\nfile2" {
		t.Errorf("Stdout = %q", ce.Stdout)
	}
	if ce.ToolUseId != "cmd-1" {
		t.Errorf("ToolUseId = %q", ce.ToolUseId)
	}
}

// TestCodexParseCommandExecutionStarted 无 exit_code → tool.use + command.exec(started)
func TestCodexParseCommandExecutionStarted(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"command_execution","id":"cmd-2","command":"sleep 10"}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1)
	// 期望：tool.use + command.exec(started, exitCode=nil)
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(evs), evs)
	}
	if _, ok := evs[0].(types.ToolUseEvent); !ok {
		t.Errorf("events[0] should be ToolUseEvent, got %T", evs[0])
	}
	ce, ok := evs[1].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("events[1] should be CommandExecEvent, got %T", evs[1])
	}
	if ce.ExitCode != nil {
		t.Errorf("ExitCode should be nil for started command, got %v", *ce.ExitCode)
	}
}

// TestCodexParseCommandExecutionError exit_code != 0 → isError=true
func TestCodexParseCommandExecutionError(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"command_execution","id":"cmd-e","command":"bad","exit_code":127,"stderr":"not found"}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1)
	tr := evs[1].(types.ToolResultEvent)
	if !tr.IsError {
		t.Errorf("IsError should be true for non-zero exit")
	}
	ce := evs[2].(types.CommandExecEvent)
	if ce.ExitCode == nil || *ce.ExitCode != 127 {
		t.Errorf("ExitCode = %v, want 127", ce.ExitCode)
	}
	if ce.Stderr != "not found" {
		t.Errorf("Stderr = %q", ce.Stderr)
	}
}

// TestCodexParseCommandExecutionCmdObject command 字段为 {cmd:...} 对象形式
func TestCodexParseCommandExecutionCmdObject(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"command_execution","id":"x","command":{"cmd":"echo"},"args":["hi"],"exit_code":0}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1)
	ce := evs[2].(types.CommandExecEvent)
	if ce.Command != "echo hi" {
		t.Errorf("Command = %q, want 'echo hi'", ce.Command)
	}
}

// TestCodexParseFileChange item.completed(file_change) → 多个 file.change
func TestCodexParseFileChange(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"file_change","id":"fc-1","changes":[{"path":"/a.txt","action":"create","added_lines":5},{"path":"/b.txt","action":"delete","removed_lines":3}]}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 100)
	if len(evs) != 2 {
		t.Fatalf("expected 2 file.change events, got %d", len(evs))
	}
	fc1 := evs[0].(types.FileChangeEvent)
	if fc1.Change.Path != "/a.txt" || fc1.Change.Action != "create" {
		t.Errorf("fc1 = %+v", fc1.Change)
	}
	if fc1.Change.AddedLines == nil || *fc1.Change.AddedLines != 5 {
		t.Errorf("fc1.AddedLines = %v", fc1.Change.AddedLines)
	}
	fc2 := evs[1].(types.FileChangeEvent)
	if fc2.Change.Path != "/b.txt" || fc2.Change.Action != "delete" {
		t.Errorf("fc2 = %+v", fc2.Change)
	}
	if fc2.ToolUseId != "fc-1" {
		t.Errorf("fc2.ToolUseId = %q", fc2.ToolUseId)
	}
}

// TestCodexParseTodoList item.completed(todo_list) → todo.update
func TestCodexParseTodoList(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"todo_list","todos":[{"content":"a","status":"completed"},{"content":"b","status":"in_progress"}]}}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	tu, ok := evs[0].(types.TodoUpdateEvent)
	if !ok {
		t.Fatalf("expected TodoUpdateEvent, got %T", evs[0])
	}
	if len(tu.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(tu.Todos))
	}
	if tu.Todos[0].Status != "completed" {
		t.Errorf("todo[0].Status = %q", tu.Todos[0].Status)
	}
}

// TestCodexParseItemUnknownType item.completed 未知子类型 → nil
func TestCodexParseItemUnknownType(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	line := `{"type":"item.completed","item":{"type":"some_new_type"}}`
	if evs := (&CodexAdapter{}).ParseLine(line, ctx, 1); evs != nil {
		t.Errorf("unknown item type should return nil, got %v", evs)
	}
}

// TestCodexParseTurnCompleted turn.completed → message + streaming.done + session.end
func TestCodexParseTurnCompleted(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	ctx.CurrentMessageId = "msg-1"
	ctx.CurrentRole = "assistant"
	ctx.CurrentBlocks = []types.ContentBlock{types.TextBlock{Type: "text", Text: "final"}}

	line := `{"type":"turn.completed","usage":{"input_tokens":50,"output_tokens":30,"cost_usd":0.02},"duration_ms":1000,"num_turns":1}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 200)
	// 期望：message + streaming.done + session.end
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(evs), evs)
	}
	msg, ok := evs[0].(types.MessageEvent)
	if !ok {
		t.Fatalf("events[0] should be MessageEvent, got %T", evs[0])
	}
	if msg.Role != "assistant" || len(msg.Blocks) != 1 {
		t.Errorf("MessageEvent = %+v", msg)
	}
	sd, ok := evs[1].(types.StreamingDoneEvent)
	if !ok {
		t.Fatalf("events[1] should be StreamingDoneEvent, got %T", evs[1])
	}
	if sd.MessageId != "msg-1" {
		t.Errorf("MessageId = %q", sd.MessageId)
	}
	se, ok := evs[2].(types.SessionEndEvent)
	if !ok {
		t.Fatalf("events[2] should be SessionEndEvent, got %T", evs[2])
	}
	if se.Stats == nil {
		t.Fatalf("Stats should not be nil")
	}
	if se.Stats.InputTokens == nil || *se.Stats.InputTokens != 50 {
		t.Errorf("InputTokens = %v", se.Stats.InputTokens)
	}
	if se.Stats.CostUsd == nil || *se.Stats.CostUsd != 0.02 {
		t.Errorf("CostUsd = %v", se.Stats.CostUsd)
	}
	// 清空累积
	if ctx.CurrentMessageId != "" || len(ctx.CurrentBlocks) != 0 {
		t.Errorf("accumulation should be cleared after turn.completed")
	}
}

// TestCodexParseTurnCompletedNoBlocks 无累积 blocks → 只有 streaming.done + session.end
func TestCodexParseTurnCompletedNoBlocks(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliCodex, "")
	ctx.CurrentMessageId = "msg-1"
	// CurrentBlocks 为空
	line := `{"type":"turn.completed"}`
	evs := (&CodexAdapter{}).ParseLine(line, ctx, 1)
	// streaming.done + session.end（无 message）
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (no message), got %d: %v", len(evs), evs)
	}
	if _, ok := evs[0].(types.StreamingDoneEvent); !ok {
		t.Errorf("events[0] should be StreamingDoneEvent, got %T", evs[0])
	}
	if _, ok := evs[1].(types.SessionEndEvent); !ok {
		t.Errorf("events[1] should be SessionEndEvent, got %T", evs[1])
	}
}

// ==== 辅助函数 ====

func TestGetItemTypeSchemaDrift(t *testing.T) {
	cases := []struct {
		item string
		want string
	}{
		{`{"type":"agent_message"}`, "agent_message"},
		{`{"type":"assistant_message"}`, "agent_message"}, // 旧版归一
		{`{"type":"reasoning"}`, "reasoning"},
		{`{"item_type":"command_execution"}`, "command_execution"}, // 旧字段名
		{`{}`, ""},                                                  // 都没有
		{`{"type":"file_change"}`, "file_change"},
	}
	for _, c := range cases {
		if got := getItemType(json.RawMessage(c.item)); got != c.want {
			t.Errorf("getItemType(%s) = %q, want %q", c.item, got, c.want)
		}
	}
}

func TestExtractCommand(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`"ls -la"`, "ls -la"},    // 字符串形式
		{`{"cmd":"echo"}`, "echo"}, // 对象形式
		{`{}`, ""},                 // 空对象
		{``, ""},                   // 空
	}
	for _, c := range cases {
		var raw json.RawMessage
		if c.raw != "" {
			raw = json.RawMessage(c.raw)
		}
		if got := extractCommand(raw); got != c.want {
			t.Errorf("extractCommand(%s) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestNormalizeFileAction(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"create", "create"},
		{"edit", "edit"},
		{"delete", "delete"},
		{"write", "create"},   // 别名
		{"modify", "edit"},    // 别名
		{"update", "edit"},    // 别名
		{"remove", "delete"},  // 别名
		{"unknown", "edit"},   // 默认
		{"", "edit"},          // 空默认
	}
	for _, c := range cases {
		if got := normalizeFileAction(c.in); got != c.want {
			t.Errorf("normalizeFileAction(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
