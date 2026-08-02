package adapter

import (
	"strings"
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// TestTraeKind trae 适配器 Kind() 返回 trae
func TestTraeKind(t *testing.T) {
	if (&TraeAdapter{}).Kind() != types.CliTrae {
		t.Errorf("Kind() != trae")
	}
}

// TestTraeBuildStartCommand command 应为 "trae"，参数同 claude-code
func TestTraeBuildStartCommand(t *testing.T) {
	cmd := (&TraeAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:   "/c",
		Model: "claude-sonnet-4",
	})
	if cmd.Command != "trae" {
		t.Errorf("Command = %q, want trae", cmd.Command)
	}
	// 验证参数与 claude-code 一致（除 command 名）
	ccCmd := (&ClaudeCodeAdapter{}).BuildStartCommand(BuildCommandOpts{
		Cwd:   "/c",
		Model: "claude-sonnet-4",
	})
	if len(cmd.Args) != len(ccCmd.Args) {
		t.Fatalf("trae args len = %d, claude-code args len = %d", len(cmd.Args), len(ccCmd.Args))
	}
	for i, a := range cmd.Args {
		if a != ccCmd.Args[i] {
			t.Errorf("arg[%d]: trae=%q claude-code=%q", i, a, ccCmd.Args[i])
		}
	}
}

// TestTraeBuildUserInput 继承 claude-code 的 stdin 多轮输入
func TestTraeBuildUserInput(t *testing.T) {
	got := (&TraeAdapter{}).BuildUserInput("test")
	if !strings.Contains(got, `"text":"test"`) {
		t.Errorf("BuildUserInput should inherit claude-code format, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("BuildUserInput should end with newline")
	}
}

// TestTraeInheritsParseLine trae 应能解析 claude-code 格式的行（继承 ParseLine）
func TestTraeInheritsParseLine(t *testing.T) {
	ctx := NewParseContext("sess-1", "/c", types.CliTrae, "")
	a := &TraeAdapter{}

	// system init
	line := `{"type":"system","subtype":"init","session_id":"cli-x","model":"claude-sonnet-4","cwd":"/c"}`
	evs := a.ParseLine(line, ctx, 100)
	if len(evs) != 1 {
		t.Fatalf("trae should inherit claude-code ParseLine, got %d events", len(evs))
	}
	si, ok := evs[0].(types.SessionInitEvent)
	if !ok {
		t.Fatalf("expected SessionInitEvent, got %T", evs[0])
	}
	// 注意：trae 解析时 ctx.Cli 是 trae，所以 session.init 的 cli 字段是 trae
	if si.Cli != "trae" {
		t.Errorf("Cli = %q, want trae (uses ctx.Cli)", si.Cli)
	}
	if si.CliSessionId != "cli-x" {
		t.Errorf("CliSessionId = %q", si.CliSessionId)
	}
}

// TestTraeInheritsFullFlow 完整继承：system → assistant text → result
func TestTraeInheritsFullFlow(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliTrae, "")
	a := &TraeAdapter{}

	// system init
	evs := a.ParseLine(`{"type":"system","subtype":"init","session_id":"x","cwd":"/c"}`, ctx, 1)
	if len(evs) != 1 {
		t.Fatalf("system init: got %d events", len(evs))
	}

	// assistant text
	evs = a.ParseLine(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`, ctx, 2)
	if len(evs) != 1 {
		t.Fatalf("assistant text: got %d events", len(evs))
	}
	if _, ok := evs[0].(types.MessageEvent); !ok {
		t.Errorf("expected MessageEvent")
	}

	// result success
	evs = a.ParseLine(`{"type":"result","subtype":"success","is_error":false,"session_id":"x"}`, ctx, 3)
	if len(evs) != 1 {
		t.Fatalf("result: got %d events", len(evs))
	}
	if _, ok := evs[0].(types.SessionEndEvent); !ok {
		t.Errorf("expected SessionEndEvent")
	}
}
