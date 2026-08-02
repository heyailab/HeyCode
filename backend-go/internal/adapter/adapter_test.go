package adapter

import (
	"testing"

	"github.com/heycode/backend-go/internal/types"
)

// TestGet 验证工厂函数对所有 CliKind 的分发（§2.6.3 适配器矩阵）。
func TestGet(t *testing.T) {
	cases := []struct {
		kind     types.CliKind
		wantKind types.CliKind
		wantErr  bool
	}{
		{types.CliClaudeCode, types.CliClaudeCode, false},
		{types.CliTrae, types.CliTrae, false},
		{types.CliCodex, types.CliCodex, false},
		{types.CliOpencode, types.CliOpencode, false},
		{types.CliPty, types.CliPty, false},
		{types.CliGemini, "", true},  // 无专用适配器，引导 pty 兜底
		{types.CliLingma, "", true},  // 无专用适配器，引导 pty 兜底
		{"unknown-cli", "", true},   // 未知 kind
	}
	for _, c := range cases {
		t.Run(string(c.kind), func(t *testing.T) {
			a, err := Get(c.kind)
			if c.wantErr {
				if err == nil {
					t.Fatalf("Get(%q) expected error, got nil", c.kind)
				}
				if a != nil {
					t.Fatalf("Get(%q) expected nil adapter on error", c.kind)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get(%q) unexpected error: %v", c.kind, err)
			}
			if a.Kind() != c.wantKind {
				t.Errorf("Get(%q).Kind() = %q, want %q", c.kind, a.Kind(), c.wantKind)
			}
		})
	}
}

// TestNewParseContext 验证 ParseContext 初始化。
func TestNewParseContext(t *testing.T) {
	ctx := NewParseContext("sess-1", "/tmp/proj", types.CliCodex, "gpt-5")
	if ctx.SessionId != "sess-1" || ctx.Cwd != "/tmp/proj" || ctx.Cli != types.CliCodex || ctx.Model != "gpt-5" {
		t.Errorf("NewParseContext fields not set correctly: %+v", ctx)
	}
	if ctx.ToolUseIndex == nil {
		t.Errorf("ToolUseIndex should be initialized")
	}
	if len(ctx.PendingToolUseIds) != 0 {
		t.Errorf("PendingToolUseIds should be empty initially")
	}
}

// TestParseContextToolUseQueue 验证 FIFO 队列 + 索引的入队/出队/查找/清理。
func TestParseContextToolUseQueue(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")

	// 入队两个
	ctx.EnqueueToolUse("id-1", ToolUseInfo{Name: "Bash", Input: []byte(`{}`)})
	ctx.EnqueueToolUse("id-2", ToolUseInfo{Name: "Write", Input: []byte(`{}`)})

	if len(ctx.PendingToolUseIds) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(ctx.PendingToolUseIds))
	}

	// 查找
	if info, ok := ctx.LookupToolUse("id-1"); !ok || info.Name != "Bash" {
		t.Errorf("LookupToolUse(id-1) failed: %+v ok=%v", info, ok)
	}

	// FIFO 出队：id-1 先出
	if got := ctx.ShiftToolUse(); got != "id-1" {
		t.Errorf("ShiftToolUse() = %q, want id-1", got)
	}
	if got := ctx.ShiftToolUse(); got != "id-2" {
		t.Errorf("ShiftToolUse() second = %q, want id-2", got)
	}
	// 空队列出队返回 ""
	if got := ctx.ShiftToolUse(); got != "" {
		t.Errorf("ShiftToolUse() empty = %q, want empty", got)
	}

	// 索引仍在（Shift 不删索引）
	if _, ok := ctx.LookupToolUse("id-1"); !ok {
		t.Errorf("LookupToolUse(id-1) should still exist after Shift (Forget cleans index)")
	}

	// Forget 清理索引 + 从队列移除
	ctx.EnqueueToolUse("id-3", ToolUseInfo{Name: "Edit"})
	ctx.ForgetToolUse("id-3")
	if _, ok := ctx.LookupToolUse("id-3"); ok {
		t.Errorf("LookupToolUse(id-3) should be gone after Forget")
	}
	if len(ctx.PendingToolUseIds) != 0 {
		t.Errorf("PendingToolUseIds should be empty after Forget, got %d", len(ctx.PendingToolUseIds))
	}
}

// TestParseContextForgetMiddle 验证 Forget 从队列中间移除。
func TestParseContextForgetMiddle(t *testing.T) {
	ctx := NewParseContext("s", "/c", types.CliClaudeCode, "")
	ctx.EnqueueToolUse("a", ToolUseInfo{})
	ctx.EnqueueToolUse("b", ToolUseInfo{})
	ctx.EnqueueToolUse("c", ToolUseInfo{})

	ctx.ForgetToolUse("b") // 移除中间元素
	if len(ctx.PendingToolUseIds) != 2 {
		t.Fatalf("expected 2 pending after Forget middle, got %d", len(ctx.PendingToolUseIds))
	}
	if ctx.PendingToolUseIds[0] != "a" || ctx.PendingToolUseIds[1] != "c" {
		t.Errorf("queue = %v, want [a c]", ctx.PendingToolUseIds)
	}
}
