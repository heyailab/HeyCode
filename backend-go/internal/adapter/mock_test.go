package adapter

import (
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// TestMockKind mock 复用 claude-code 协议，Kind() 返回 claude-code
func TestMockKind(t *testing.T) {
	if (&MockAdapter{}).Kind() != types.CliClaudeCode {
		t.Errorf("Kind() = %q, want claude-code (mock reuses claude-code protocol)", (&MockAdapter{}).Kind())
	}
}

// TestMockBuildStartCommand mock 不实际启动进程，返回占位
func TestMockBuildStartCommand(t *testing.T) {
	cmd := (&MockAdapter{}).BuildStartCommand(BuildCommandOpts{Cwd: "/c", Prompt: "p"})
	if cmd.Command != "mock-claude" {
		t.Errorf("Command = %q, want mock-claude", cmd.Command)
	}
}

// TestMockGenerateTimeline 时间线应产出 6 行 claude-code stream-json
func TestMockGenerateTimeline(t *testing.T) {
	timeline := (&MockAdapter{}).GenerateTimeline("fix bug")
	if len(timeline) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(timeline))
	}
	// 每行应为合法 JSON
	for i, line := range timeline {
		if line == "" {
			t.Errorf("line[%d] is empty", i)
		}
		// 简单校验 JSON 起止
		if line[0] != '{' || line[len(line)-1] != '}' {
			t.Errorf("line[%d] not JSON object: %s", i, line)
		}
	}

	// 第一行应是 system init
	if timeline[0] != `{"type":"system","subtype":"init","session_id":"mock-session-001","model":"claude-sonnet-4","cwd":"/tmp/mock"}` {
		t.Errorf("line[0] unexpected: %s", timeline[0])
	}
	// 最后一行应是 result success
	if timeline[5] == "" {
		t.Errorf("last line empty")
	}
}

// TestMockTimelineRoundTrip 端到端：GenerateTimeline → ParseLine → 事件序列
// 验证 mock 时间线喂给 ParseLine 产出的事件流符合预期
func TestMockTimelineRoundTrip(t *testing.T) {
	a := &MockAdapter{}
	timeline := a.GenerateTimeline("test prompt")
	ctx := NewParseContext("sess-mock", "/tmp/mock", types.CliClaudeCode, "")

	var allEvents []types.UnifiedEvent
	for i, line := range timeline {
		evs := a.ParseLine(line, ctx, int64(i*100))
		allEvents = append(allEvents, evs...)
	}

	// 期望事件序列：
	// 1. system init → session.init
	// 2. assistant text "我来帮你处理：test prompt" → message(assistant, text)
	// 3. assistant tool_use(Bash) → tool.use + message
	//    （Bash 不衍生 file.change）
	// 4. user tool_result → tool.result + command.exec(Bash 衍生) + message
	// 5. assistant text "目录为空，任务完成。" → message
	// 6. result success → session.end

	// 验证关键事件存在
	var hasSessionInit, hasToolUse, hasToolResult, hasCommandExec, hasSessionEnd bool
	var messageCount int
	var commandOutput string
	var firstAssistantText string // 第一条 assistant 消息的文本（应含 prompt）
	for _, e := range allEvents {
		switch ev := e.(type) {
		case types.SessionInitEvent:
			hasSessionInit = true
		case types.ToolUseEvent:
			hasToolUse = true
		case types.ToolResultEvent:
			hasToolResult = true
		case types.CommandExecEvent:
			hasCommandExec = true
			if ev.Command == "ls -la /tmp/mock" {
				commandOutput = ev.Stdout
			}
		case types.MessageEvent:
			messageCount++
			// 第一条 assistant 消息应是 "我来帮你处理：test prompt"
			if firstAssistantText == "" && ev.Role == "assistant" {
				if len(ev.Blocks) > 0 {
					if tb, ok := ev.Blocks[0].(types.TextBlock); ok {
						firstAssistantText = tb.Text
					}
				}
			}
		case types.SessionEndEvent:
			hasSessionEnd = true
		}
	}

	if !hasSessionInit {
		t.Errorf("missing session.init event")
	}
	if !hasToolUse {
		t.Errorf("missing tool.use event")
	}
	if !hasToolResult {
		t.Errorf("missing tool.result event")
	}
	if !hasCommandExec {
		t.Errorf("missing command.exec event (Bash derivation)")
	}
	if !hasSessionEnd {
		t.Errorf("missing session.end event")
	}
	// 期望 4 条 message（assistant text x3 + user tool_result x1）
	//   line1: assistant "我来帮你处理：test prompt" → message
	//   line2: assistant tool_use(Bash) → tool.use + message
	//   line3: user tool_result → tool.result + command.exec + message
	//   line4: assistant "目录为空，任务完成。" → message
	if messageCount != 4 {
		t.Errorf("message count = %d, want 4", messageCount)
	}
	// 验证首条 assistant 消息含 prompt（防止 JSON 解析失败降级为 command.exec）
	if firstAssistantText != "我来帮你处理：test prompt" {
		t.Errorf("first assistant text = %q, want %q", firstAssistantText, "我来帮你处理：test prompt")
	}
	// 验证 Bash 输出被正确衍生到 command.exec.stdout
	if commandOutput == "" {
		t.Errorf("command.exec stdout empty, expected tool_result content")
	}
}

// TestMockGenerateTimelinePromptEscape prompt 含特殊字符应被正确 JSON 转义
func TestMockGenerateTimelinePromptEscape(t *testing.T) {
	timeline := (&MockAdapter{}).GenerateTimeline(`hello "world" \n`)
	// 第二行应包含转义后的 prompt
	line1 := timeline[1]
	// 应为合法 JSON
	// 验证 prompt 被嵌入到 text 字段
	// 简单检查：应包含 "我来帮你处理：" 前缀
	// 用 ParseLine 解析验证
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	evs := (&MockAdapter{}).ParseLine(line1, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	msg := evs[0].(types.MessageEvent)
	tb := msg.Blocks[0].(types.TextBlock)
	expected := `我来帮你处理：hello "world" \n`
	if tb.Text != expected {
		t.Errorf("text = %q, want %q", tb.Text, expected)
	}
}
