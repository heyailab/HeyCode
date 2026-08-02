package adapter

import (
	"encoding/json"
	"fmt"

	"github.com/heycode/backend-go/internal/types"
)

// MockAdapter 复用 claude-code 适配器的解析逻辑，但实现 TimelineGenerator 接口。
// runner 检测到该接口存在即走 Mock 路径：不连 SSH，用 25ms delay 逐行喂 ParseLine。
//
// 见 §4.12：仅 MOCK_CLI=1 且 cli=claude-code 时生效。
// Mock timeline 产出 claude-code stream-json 模拟序列：
//   system init → assistant text → assistant tool_use(Bash) → user tool_result → assistant text → result success
type MockAdapter struct {
	ClaudeCodeAdapter
}

// Kind 返回 claude-code（mock 复用 claude-code 协议）。
func (a *MockAdapter) Kind() types.CliKind { return types.CliClaudeCode }

// BuildStartCommand mock 模式不实际启动进程，返回占位命令。
func (a *MockAdapter) BuildStartCommand(opts BuildCommandOpts) StartCommand {
	return StartCommand{Command: "mock-claude", Args: nil}
}

// GenerateTimeline 产出模拟 claude-code stream-json 的行序列。
// runner 用 25ms delay 逐行喂回 ParseLine，验证事件流端到端。
func (a *MockAdapter) GenerateTimeline(prompt string) []string {
	// 预拼接完整文本再 JSON 编码，避免引号嵌套产生非法 JSON
	// （jsonString 返回值已含外层引号，模板里不能再加引号）
	textJSON := jsonString("我来帮你处理：" + prompt)
	return []string{
		// 1. system init
		`{"type":"system","subtype":"init","session_id":"mock-session-001","model":"claude-sonnet-4","cwd":"/tmp/mock"}`,

		// 2. assistant 文本回复（textJSON 已含引号，模板用 "text":%s 不再加引号）
		fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":%s}]}}`, textJSON),

		// 3. assistant tool_use(Bash)
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_mock_bash","name":"Bash","input":{"command":"ls -la /tmp/mock"}}]}}`,

		// 4. user tool_result
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_mock_bash","content":"total 0\ndrwxr-xr-x 2 root root 40 Aug 1 00:00 ."}]}}`,

		// 5. assistant 总结文本
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"目录为空，任务完成。"}]}}`,

		// 6. result success
		`{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"mock-session-001","total_cost_usd":0.01,"duration_ms":1500,"num_turns":2,"usage_tokens":120,"output_tokens":80}`,
	}
}

// jsonString 把字符串转为 JSON 字符串字面量（含引号）。
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
