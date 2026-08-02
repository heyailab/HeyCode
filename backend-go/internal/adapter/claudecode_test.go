package adapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// ==== Kind / BuildStartCommand / BuildUserInput ====

func TestClaudeCodeKind(t *testing.T) {
	if (&ClaudeCodeAdapter{}).Kind() != types.CliClaudeCode {
		t.Errorf("Kind() != claude-code")
	}
}

func TestClaudeCodeBuildStartCommandBasic(t *testing.T) {
	cmd := (&ClaudeCodeAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd: "/home/user/proj",
	})
	if cmd.Command != "claude" {
		t.Errorf("Command = %q, want claude", cmd.Command)
	}
	// 必含固定参数
	wantArgs := []string{"-p", "--output-format", "stream-json", "--input-format", "stream-json", "--verbose", "--cd", "/home/user/proj"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("Args = %v, want %v", cmd.Args, wantArgs)
	}
	for i, w := range wantArgs {
		if cmd.Args[i] != w {
			t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], w)
		}
	}
	// prompt 不应进命令行（走 stdin）
	for _, a := range cmd.Args {
		if strings.Contains(a, "prompt") {
			t.Errorf("prompt should not appear in args: %v", cmd.Args)
		}
	}
}

func TestClaudeCodeBuildStartCommandWithOpts(t *testing.T) {
	cmd := (&ClaudeCodeAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:                "/c",
		Model:              "claude-sonnet-4",
		ResumeCliSessionId: "sess-abc",
		AllowedTools:       []string{"Bash", "Read"},
	})
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--model claude-sonnet-4") {
		t.Errorf("missing --model: %v", cmd.Args)
	}
	if !strings.Contains(joined, "--resume sess-abc") {
		t.Errorf("missing --resume: %v", cmd.Args)
	}
	if !strings.Contains(joined, "--allowedTools Bash,Read") {
		t.Errorf("missing --allowedTools: %v", cmd.Args)
	}
}

func TestClaudeCodeBuildUserInput(t *testing.T) {
	got := (&ClaudeCodeAdapter{}).BuildUserInput("hello \"world\"\n")
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("BuildUserInput should end with newline")
	}
	// 应为合法 JSON
	line := strings.TrimRight(got, "\n")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("BuildUserInput not valid JSON: %v\n%s", err, line)
	}
	if obj["type"] != "user" {
		t.Errorf("type = %v, want user", obj["type"])
	}
	// prompt 应被正确转义（含引号和换行）
	msg := obj["message"].(map[string]interface{})
	content := msg["content"].([]interface{})
	block := content[0].(map[string]interface{})
	if block["text"] != "hello \"world\"\n" {
		t.Errorf("text = %q, want hello \"world\"\\n", block["text"])
	}
}

// ==== ParseLine ====

func TestClaudeCodeParseEmptyLine(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	if evs := a.ParseLine("   \n", ctx, 1); evs != nil {
		t.Errorf("empty line should return nil, got %v", evs)
	}
}

func TestClaudeCodeParseNonJSON(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	evs := a.ParseLine("not a json line", ctx, 100)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event for non-JSON line, got %d", len(evs))
	}
	ce, ok := evs[0].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("expected CommandExecEvent, got %T", evs[0])
	}
	if ce.Command != "not a json line" {
		t.Errorf("Command = %q", ce.Command)
	}
	if ce.Stdout != "not a json line" {
		t.Errorf("Stdout = %q", ce.Stdout)
	}
	if ce.Cwd != "/c" {
		t.Errorf("Cwd = %q, want /c", ce.Cwd)
	}
}

func TestClaudeCodeParseUnknownType(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	if evs := a.ParseLine(`{"type":"ping"}`, ctx, 1); evs != nil {
		t.Errorf("unknown type should return nil, got %v", evs)
	}
}

// TestClaudeCodeParseSystemInit system/init → session.init
func TestClaudeCodeParseSystemInit(t *testing.T) {
	ctx := NewParseContext("sess-1", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"system","subtype":"init","session_id":"cli-sess-xyz","model":"claude-sonnet-4","cwd":"/home/u/proj"}`
	evs := a.ParseLine(line, ctx, 1000)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	si, ok := evs[0].(types.SessionInitEvent)
	if !ok {
		t.Fatalf("expected SessionInitEvent, got %T", evs[0])
	}
	if si.SessionId != "sess-1" {
		t.Errorf("SessionId = %q, want sess-1", si.SessionId)
	}
	if si.CliSessionId != "cli-sess-xyz" {
		t.Errorf("CliSessionId = %q, want cli-sess-xyz", si.CliSessionId)
	}
	if si.Cli != "claude-code" {
		t.Errorf("Cli = %q", si.Cli)
	}
	if si.Model != "claude-sonnet-4" {
		t.Errorf("Model = %q", si.Model)
	}
	if si.Cwd != "/home/u/proj" {
		t.Errorf("Cwd = %q", si.Cwd)
	}
	if si.Timestamp != 1000 {
		t.Errorf("Timestamp = %d", si.Timestamp)
	}
}

// TestClaudeCodeParseSystemInitModelFromCtx ctx.Model 优先于 sys.Model
func TestClaudeCodeParseSystemInitModelFromCtx(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "user-chosen-model")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"system","subtype":"init","session_id":"x","model":"claude-default","cwd":"/c"}`
	evs := a.ParseLine(line, ctx, 1)
	si := evs[0].(types.SessionInitEvent)
	if si.Model != "user-chosen-model" {
		t.Errorf("Model = %q, want user-chosen-model (ctx priority)", si.Model)
	}
}

// TestClaudeCodeParseSystemNonInit 非 init 子类型应被忽略
func TestClaudeCodeParseSystemNonInit(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	if evs := a.ParseLine(`{"type":"system","subtype":"other"}`, ctx, 1); len(evs) != 0 {
		t.Errorf("non-init system should return nil, got %v", evs)
	}
}

// TestClaudeCodeParseAssistantText assistant 纯文本 → 1 个 message 事件
func TestClaudeCodeParseAssistantText(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}`
	evs := a.ParseLine(line, ctx, 100)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (message), got %d: %v", len(evs), evs)
	}
	msg, ok := evs[0].(types.MessageEvent)
	if !ok {
		t.Fatalf("expected MessageEvent, got %T", evs[0])
	}
	if msg.Role != "assistant" {
		t.Errorf("Role = %q", msg.Role)
	}
	if len(msg.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(msg.Blocks))
	}
	tb, ok := msg.Blocks[0].(types.TextBlock)
	if !ok {
		t.Fatalf("expected TextBlock, got %T", msg.Blocks[0])
	}
	if tb.Text != "hello" {
		t.Errorf("Text = %q", tb.Text)
	}
}

// TestClaudeCodeParseAssistantStringContent content 为纯字符串
func TestClaudeCodeParseAssistantStringContent(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"assistant","message":{"role":"assistant","content":"plain string reply"}}`
	evs := a.ParseLine(line, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	msg := evs[0].(types.MessageEvent)
	tb := msg.Blocks[0].(types.TextBlock)
	if tb.Text != "plain string reply" {
		t.Errorf("Text = %q", tb.Text)
	}
}

// TestClaudeCodeParseAssistantBashToolUse assistant Bash tool_use → tool.use + message（无衍生）
func TestClaudeCodeParseAssistantBashToolUse(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls -la"}}]}}`
	evs := a.ParseLine(line, ctx, 100)
	// 期望：tool.use + message（Bash 不衍生 file.change）
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (tool.use + message), got %d: %v", len(evs), evs)
	}
	tu, ok := evs[0].(types.ToolUseEvent)
	if !ok {
		t.Fatalf("events[0] should be ToolUseEvent, got %T", evs[0])
	}
	if tu.ToolUseId != "toolu_1" || tu.ToolName != "Bash" {
		t.Errorf("ToolUseEvent = %+v", tu)
	}
	// 验证 input
	var input map[string]string
	_ = json.Unmarshal(tu.Input, &input)
	if input["command"] != "ls -la" {
		t.Errorf("input.command = %q", input["command"])
	}
	// 应入队
	if len(ctx.PendingToolUseIds) != 1 || ctx.PendingToolUseIds[0] != "toolu_1" {
		t.Errorf("PendingToolUseIds = %v, want [toolu_1]", ctx.PendingToolUseIds)
	}
	// events[1] 是 message
	if _, ok := evs[1].(types.MessageEvent); !ok {
		t.Errorf("events[1] should be MessageEvent, got %T", evs[1])
	}
}

// TestClaudeCodeParseAssistantWriteToolUse assistant Write → tool.use + file.change(create) + message
func TestClaudeCodeParseAssistantWriteToolUse(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_w","name":"Write","input":{"file_path":"/tmp/new.txt"}}]}}`
	evs := a.ParseLine(line, ctx, 1)
	// 期望：tool.use + file.change + message
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(evs), evs)
	}
	fc, ok := evs[1].(types.FileChangeEvent)
	if !ok {
		t.Fatalf("events[1] should be FileChangeEvent, got %T", evs[1])
	}
	if fc.Change.Path != "/tmp/new.txt" || fc.Change.Action != "create" {
		t.Errorf("FileChange = %+v", fc.Change)
	}
	if fc.ToolUseId != "tu_w" {
		t.Errorf("ToolUseId = %q", fc.ToolUseId)
	}
}

// TestClaudeCodeParseAssistantEditToolUse Edit → action=edit
func TestClaudeCodeParseAssistantEditToolUse(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_e","name":"Edit","input":{"file_path":"/tmp/edit.txt"}}]}}`
	evs := a.ParseLine(line, ctx, 1)
	fc := evs[1].(types.FileChangeEvent)
	if fc.Change.Action != "edit" {
		t.Errorf("Edit action = %q, want edit", fc.Change.Action)
	}
}

// TestClaudeCodeParseAssistantTodoWrite TodoWrite → tool.use + todo.update + message
func TestClaudeCodeParseAssistantTodoWrite(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_td","name":"TodoWrite","input":{"todos":[{"content":"step 1","status":"in_progress"},{"content":"step 2","status":"pending"}]}}]}}`
	evs := a.ParseLine(line, ctx, 1)
	// 期望：tool.use + todo.update + message
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(evs), evs)
	}
	tu, ok := evs[1].(types.TodoUpdateEvent)
	if !ok {
		t.Fatalf("events[1] should be TodoUpdateEvent, got %T", evs[1])
	}
	if len(tu.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(tu.Todos))
	}
	if tu.Todos[0].Content != "step 1" || tu.Todos[0].Status != "in_progress" {
		t.Errorf("todo[0] = %+v", tu.Todos[0])
	}
	if tu.Todos[1].Status != "pending" {
		t.Errorf("todo[1].Status = %q", tu.Todos[1].Status)
	}
}

// TestClaudeCodeParseUserToolResultBash user Bash tool_result → tool.result + command.exec + message
func TestClaudeCodeParseUserToolResultBash(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	// 先登记 Bash tool_use（command=ls）
	ctx.EnqueueToolUse("tu_bash", ToolUseInfo{
		Name:  "Bash",
		Input: []byte(`{"command":"ls -la"}`),
	})
	line := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_bash","content":"file1\nfile2"}]}}`
	evs := a.ParseLine(line, ctx, 200)
	// 期望：tool.result + command.exec + message
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(evs), evs)
	}
	tr, ok := evs[0].(types.ToolResultEvent)
	if !ok {
		t.Fatalf("events[0] should be ToolResultEvent, got %T", evs[0])
	}
	if tr.ToolUseId != "tu_bash" {
		t.Errorf("ToolUseId = %q", tr.ToolUseId)
	}
	// output 是字符串
	var outStr string
	_ = json.Unmarshal(tr.Output, &outStr)
	if outStr != "file1\nfile2" {
		t.Errorf("Output = %q", outStr)
	}
	// command.exec
	ce, ok := evs[1].(types.CommandExecEvent)
	if !ok {
		t.Fatalf("events[1] should be CommandExecEvent, got %T", evs[1])
	}
	if ce.Command != "ls -la" {
		t.Errorf("Command = %q", ce.Command)
	}
	if ce.ExitCode == nil || *ce.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", ce.ExitCode)
	}
	if ce.Stdout != "file1\nfile2" {
		t.Errorf("Stdout = %q", ce.Stdout)
	}
	if ce.ToolUseId != "tu_bash" {
		t.Errorf("ToolUseId = %q", ce.ToolUseId)
	}
	// 处理完应清理登记
	if _, ok := ctx.LookupToolUse("tu_bash"); ok {
		t.Errorf("tool_use should be forgotten after tool_result")
	}
}

// TestClaudeCodeParseUserToolResultBashError Bash 失败 → exit=1, stderr
func TestClaudeCodeParseUserToolResultBashError(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	ctx.EnqueueToolUse("tu_bash", ToolUseInfo{
		Name:  "Bash",
		Input: []byte(`{"command":"bad-cmd"}`),
	})
	line := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_bash","content":"command not found","is_error":true}]}}`
	evs := a.ParseLine(line, ctx, 1)
	ce := evs[1].(types.CommandExecEvent)
	if ce.ExitCode == nil || *ce.ExitCode != 1 {
		t.Errorf("ExitCode = %v, want 1", ce.ExitCode)
	}
	if ce.Stdout != "" {
		t.Errorf("Stdout should be empty on error, got %q", ce.Stdout)
	}
	if ce.Stderr != "command not found" {
		t.Errorf("Stderr = %q", ce.Stderr)
	}
}

// TestClaudeCodeParseResultSuccess result success → session.end(stats)
func TestClaudeCodeParseResultSuccess(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"x","total_cost_usd":0.05,"duration_ms":2000,"num_turns":3,"usage_tokens":100,"output_tokens":50}`
	evs := a.ParseLine(line, ctx, 5000)
	// 非 error → 只有 session.end
	if len(evs) != 1 {
		t.Fatalf("expected 1 event (session.end), got %d: %v", len(evs), evs)
	}
	se, ok := evs[0].(types.SessionEndEvent)
	if !ok {
		t.Fatalf("expected SessionEndEvent, got %T", evs[0])
	}
	if se.Stats == nil {
		t.Fatalf("Stats should not be nil")
	}
	if se.Stats.CostUsd == nil || *se.Stats.CostUsd != 0.05 {
		t.Errorf("CostUsd = %v", se.Stats.CostUsd)
	}
	if se.Stats.DurationMs == nil || *se.Stats.DurationMs != 2000 {
		t.Errorf("DurationMs = %v", se.Stats.DurationMs)
	}
	if se.Stats.NumTurns == nil || *se.Stats.NumTurns != 3 {
		t.Errorf("NumTurns = %v", se.Stats.NumTurns)
	}
	if se.Stats.InputTokens == nil || *se.Stats.InputTokens != 100 {
		t.Errorf("InputTokens = %v", se.Stats.InputTokens)
	}
}

// TestClaudeCodeParseResultError result error → error(recoverable=false) + session.end
func TestClaudeCodeParseResultError(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"result","subtype":"error_max_turns","is_error":true,"result":"max turns reached","session_id":"x"}`
	evs := a.ParseLine(line, ctx, 5000)
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (error + session.end), got %d: %v", len(evs), evs)
	}
	ee, ok := evs[0].(types.ErrorEvent)
	if !ok {
		t.Fatalf("events[0] should be ErrorEvent, got %T", evs[0])
	}
	if ee.Message != "max turns reached" {
		t.Errorf("Message = %q", ee.Message)
	}
	if ee.Recoverable == nil || *ee.Recoverable != false {
		t.Errorf("Recoverable = %v, want false", ee.Recoverable)
	}
	if ee.Cli != "claude-code" {
		t.Errorf("Cli = %q", ee.Cli)
	}
	// events[1] 是 session.end
	if _, ok := evs[1].(types.SessionEndEvent); !ok {
		t.Errorf("events[1] should be SessionEndEvent, got %T", evs[1])
	}
}

// TestClaudeCodeParseResultNoStats result 无统计字段 → session.end(nil stats)
func TestClaudeCodeParseResultNoStats(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	a := &ClaudeCodeAdapter{}
	line := `{"type":"result","subtype":"success","is_error":false,"result":"ok","session_id":"x"}`
	evs := a.ParseLine(line, ctx, 1)
	se := evs[0].(types.SessionEndEvent)
	if se.Stats != nil {
		t.Errorf("Stats should be nil when no fields, got %+v", se.Stats)
	}
}
