package types

import (
	"encoding/json"
	"testing"
)

// TestEventMarshalUnmarshalRoundTrip 验证 13 种事件 Marshal → Unmarshal 往返保持类型。
func TestEventMarshalUnmarshalRoundTrip(t *testing.T) {
	exitCode := 0
	step := 2
	total := 5
	cost := 0.05
	dur := int64(1500)
	turns := 3
	inTok := 100
	outTok := 50
	prog := 60
	recoverable := false

	cases := []struct {
		name string
		ev   UnifiedEvent
	}{
		{"session.init", NewSessionInit(1000, "sess-1", "cli-x", "claude-code", "claude-sonnet-4", "/c")},
		{"message", NewMessage(2000, "assistant", []ContentBlock{
			TextBlock{Type: "text", Text: "hi"},
			ToolUseBlock{Type: "tool_use", ToolUseId: "tu1", ToolName: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
		})},
		{"streaming.delta", NewStreamingDelta(3000, "msg-1", "chunk")},
		{"streaming.done", NewStreamingDone(4000, "msg-1")},
		{"tool.use", NewToolUse(5000, "tu1", "Bash", json.RawMessage(`{"command":"pwd"}`))},
		{"tool.result", NewToolResult(6000, "tu1", json.RawMessage(`"/home"`), false)},
		{"file.change", NewFileChange(7000, FileChange{Path: "/a.txt", Action: "create", AddedLines: &step, RemovedLines: &total}, "tu2")},
		{"command.exec", NewCommandExec(8000, "ls", "/c", &exitCode, "out", "err", "tu3")},
		{"todo.update", NewTodoUpdate(9000, []TodoItem{
			{Id: "t1", Content: "step 1", Status: "completed", Progress: &prog},
			{Id: "t2", Content: "step 2", Status: "pending"},
		})},
		{"thinking", NewThinking(11000, "let me think")},
		{"progress", NewProgress(12000, &step, &total, "processing")},
		{"error", NewError(13000, "boom", &recoverable, "claude-code")},
		{"session.end", NewSessionEnd(14000, &SessionStats{
			CostUsd: &cost, DurationMs: &dur, NumTurns: &turns,
			InputTokens: &inTok, OutputTokens: &outTok,
		})},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Marshal
			data, err := MarshalEvent(c.ev)
			if err != nil {
				t.Fatalf("MarshalEvent failed: %v", err)
			}
			// 应含 type 字段
			var head struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(data, &head); err != nil {
				t.Fatalf("marshal output not valid JSON: %v", err)
			}
			if head.Type != c.ev.EventType() {
				t.Errorf("type = %q, want %q", head.Type, c.ev.EventType())
			}
			// Unmarshal
			got, err := UnmarshalEvent(data)
			if err != nil {
				t.Fatalf("UnmarshalEvent failed: %v", err)
			}
			// 类型应一致
			if got.EventType() != c.ev.EventType() {
				t.Errorf("round-trip type mismatch: got %q, want %q", got.EventType(), c.ev.EventType())
			}
			// 再 marshal 一次，结果应一致（保证往返稳定）
			data2, err := MarshalEvent(got)
			if err != nil {
				t.Fatalf("second MarshalEvent failed: %v", err)
			}
			if string(data) != string(data2) {
				t.Errorf("round-trip not stable:\n  first:  %s\n  second: %s", data, data2)
			}
		})
	}
}

// TestNewToolUseNilInput nil input → "{}"
func TestNewToolUseNilInput(t *testing.T) {
	ev := NewToolUse(1, "tu", "Bash", nil)
	data, _ := json.Marshal(ev)
	if !contains(string(data), `"input":{}`) {
		t.Errorf("nil input should marshal to {}, got %s", data)
	}
}

// TestNewToolResultNilOutput nil output → ""
func TestNewToolResultNilOutput(t *testing.T) {
	ev := NewToolResult(1, "tu", nil, false)
	data, _ := json.Marshal(ev)
	if !contains(string(data), `"output":""`) {
		t.Errorf("nil output should marshal to \"\", got %s", data)
	}
}

// TestUnmarshalEventUnknownType 未知 type 应返回错误
func TestUnmarshalEventUnknownType(t *testing.T) {
	_, err := UnmarshalEvent([]byte(`{"type":"nonexistent","timestamp":1}`))
	if err == nil {
		t.Errorf("expected error for unknown event type")
	}
}

// TestUnmarshalEventInvalidJSON 非法 JSON 应返回错误
func TestUnmarshalEventInvalidJSON(t *testing.T) {
	_, err := UnmarshalEvent([]byte(`not json`))
	if err == nil {
		t.Errorf("expected error for invalid JSON")
	}
}

// TestUnmarshalContentBlock 5 种 ContentBlock 解析
func TestUnmarshalContentBlock(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"text", `{"type":"text","text":"hello"}`},
		{"thinking", `{"type":"thinking","text":"hmm","signature":"sig"}`},
		{"image", `{"type":"image","mimeType":"image/png","dataB64":"iVBOR"}`},
		{"tool_use", `{"type":"tool_use","toolUseId":"tu1","toolName":"Bash","input":{"command":"ls"}}`},
		{"tool_result", `{"type":"tool_result","toolUseId":"tu1","output":"done","isError":false}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := UnmarshalContentBlock([]byte(c.data))
			if err != nil {
				t.Fatalf("UnmarshalContentBlock failed: %v", err)
			}
			if b.blockType() != c.name {
				t.Errorf("blockType = %q, want %q", b.blockType(), c.name)
			}
		})
	}
}

// TestUnmarshalContentBlockUnknown 未知 type 应返回错误
func TestUnmarshalContentBlockUnknown(t *testing.T) {
	if _, err := UnmarshalContentBlock([]byte(`{"type":"mystery"}`)); err == nil {
		t.Errorf("expected error for unknown content block type")
	}
}

// TestContentBlockMarshalRoundTrip ContentBlock 序列化后应保留 type 字段
func TestContentBlockMarshalRoundTrip(t *testing.T) {
	blocks := []ContentBlock{
		TextBlock{Type: "text", Text: "t"},
		ThinkingBlock{Type: "thinking", Text: "th", Signature: "s"},
		ImageBlock{Type: "image", MimeType: "image/png", DataB64: "abc"},
		ToolUseBlock{Type: "tool_use", ToolUseId: "tu", ToolName: "Bash", Input: json.RawMessage(`{}`)},
		ToolResultBlock{Type: "tool_result", ToolUseId: "tu", Output: json.RawMessage(`"out"`), IsError: true},
	}
	for i, b := range blocks {
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("block[%d] marshal failed: %v", i, err)
		}
		// type 字段应正确
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &head); err != nil {
			t.Fatalf("block[%d] not valid JSON: %v", i, err)
		}
		if head.Type != b.blockType() {
			t.Errorf("block[%d] type = %q, want %q", i, head.Type, b.blockType())
		}
	}
}

// TestSessionInitOmitEmpty CliSessionId/Model 为空时应省略
func TestSessionInitOmitEmpty(t *testing.T) {
	ev := NewSessionInit(1, "s", "", "claude-code", "", "/c")
	data, _ := json.Marshal(ev)
	s := string(data)
	if contains(s, "cliSessionId") {
		t.Errorf("empty cliSessionId should be omitted: %s", s)
	}
	if contains(s, "model") {
		t.Errorf("empty model should be omitted: %s", s)
	}
}

// TestFileChangeOmitEmpty diff/addedLines/removedLines 为空时应省略
func TestFileChangeOmitEmpty(t *testing.T) {
	ev := NewFileChange(1, FileChange{Path: "/a", Action: "create"}, "")
	data, _ := json.Marshal(ev)
	s := string(data)
	if contains(s, "diff") {
		t.Errorf("empty diff should be omitted: %s", s)
	}
	if contains(s, "addedLines") {
		t.Errorf("nil addedLines should be omitted: %s", s)
	}
	if contains(s, "toolUseId") {
		t.Errorf("empty toolUseId should be omitted: %s", s)
	}
}

// TestErrorEventRecoverableOmitEmpty recoverable 为 nil 时应省略
func TestErrorEventRecoverableOmitEmpty(t *testing.T) {
	ev := NewError(1, "msg", nil, "")
	data, _ := json.Marshal(ev)
	s := string(data)
	if contains(s, "recoverable") {
		t.Errorf("nil recoverable should be omitted: %s", s)
	}
	if contains(s, "cli") {
		t.Errorf("empty cli should be omitted: %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
