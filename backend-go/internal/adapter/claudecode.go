package adapter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/heycode/backend-go/internal/ssh"
	"github.com/heycode/backend-go/internal/types"
)

// ClaudeCodeAdapter 实现 claude-code CLI 适配器。
//
// 模式：stream-json 进程型。
//   - 命令：claude -p --output-format stream-json --input-format stream-json --verbose --cd <cwd> [opts]
//   - prompt 走 stdin（NDJSON），多轮靠 stdin 续接（--resume 可恢复会话）
//   - 每行输出是独立 JSON 对象，按 type 字段分发
//
// trae 适配器完全继承本实现，仅 Kind() 返回 trae。
type ClaudeCodeAdapter struct{}

// Kind 返回 claude-code。
func (a *ClaudeCodeAdapter) Kind() types.CliKind { return types.CliClaudeCode }

// BuildStartCommand 构造 claude-code 启动命令。
// prompt 不进命令行（走 stdin），因此不拼到 args。
func (a *ClaudeCodeAdapter) BuildStartCommand(opts BuildCommandOpts) StartCommand {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
		"--cd", opts.Cwd,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ResumeCliSessionId != "" {
		args = append(args, "--resume", opts.ResumeCliSessionId)
	}
	if len(opts.AllowedTools) > 0 {
		// --allowedTools 接受逗号分隔列表
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}
	return StartCommand{Command: "claude", Args: args}
}

// BuildUserInput 把 prompt 转为写入 stdin 的 NDJSON。
// 格式：{"type":"user","message":{"role":"user","content":[{"type":"text","text":<prompt>}]}}\n
func (a *ClaudeCodeAdapter) BuildUserInput(prompt string) string {
	// prompt 作为 JSON 字符串值嵌入，用 json.Marshal 保证转义正确
	contentJSON, _ := json.Marshal(prompt)
	return fmt.Sprintf(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":%s}]}}`+"\n", string(contentJSON))
}

// ParseLine 解析 claude-code stream-json 的一行。
//
// 行格式：{"type":"...", ...}
// type 分发：
//   - system → 子类型 init 产 session.init
//   - user/assistant → message + mapContent 衍生（tool_use/tool_result）
//   - result → 非 success 发 error + session.end(stats)
func (a *ClaudeCodeAdapter) ParseLine(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &head); err != nil {
		// 非 JSON 行：降级为 command.exec 输出（debug 用）
		return []types.UnifiedEvent{types.NewCommandExec(ts, line, ctx.Cwd, nil, line, "", "")}
	}

	switch head.Type {
	case "system":
		return a.parseSystem(line, ctx, ts)
	case "user":
		return a.parseMessage(line, ctx, ts, "user")
	case "assistant":
		return a.parseMessage(line, ctx, ts, "assistant")
	case "result":
		return a.parseResult(line, ctx, ts)
	default:
		// 未知 type：忽略（claude-code 可能输出 ping/progress 等非核心行）
		return nil
	}
}

// ---- system 解析 ----

func (a *ClaudeCodeAdapter) parseSystem(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var sys struct {
		Subtype    string `json:"subtype"`
		SessionID  string `json:"session_id"`
		Model      string `json:"model"`
		Cwd        string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(line), &sys); err != nil {
		return nil
	}
	if sys.Subtype != "init" {
		return nil
	}
	// session.init：cliSessionId = session_id，model 优先用 ctx.Model（用户指定）回退 sys.Model
	model := ctx.Model
	if model == "" {
		model = sys.Model
	}
	cwd := sys.Cwd
	if cwd == "" {
		cwd = ctx.Cwd
	}
	return []types.UnifiedEvent{types.NewSessionInit(ts, ctx.SessionId, sys.SessionID, string(ctx.Cli), model, cwd)}
}

// ---- user/assistant message 解析 ----

// claudeMessage 是 stream-json 的 user/assistant 行结构。
type claudeMessage struct {
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"` // string 或 []ContentBlock
	} `json:"message"`
}

func (a *ClaudeCodeAdapter) parseMessage(line string, ctx *ParseContext, ts int64, role string) []types.UnifiedEvent {
	var msg claudeMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil
	}
	// content 可能是纯字符串（简短回复）或数组（含 tool_use/tool_result）
	blocks := mapContent(msg.Message.Content)
	if len(blocks) == 0 {
		return nil
	}

	var events []types.UnifiedEvent

	// 衍生事件：遍历 blocks，tool_use/tool_result 衍生独立事件
	for _, b := range blocks {
		switch v := b.(type) {
		case types.ToolUseBlock:
			// tool.use 事件
			events = append(events, types.NewToolUse(ts, v.ToolUseId, v.ToolName, v.Input))
			// 入队 + 登记（用于后续 tool_result 匹配与衍生）
			ctx.EnqueueToolUse(v.ToolUseId, ToolUseInfo{Name: v.ToolName, Input: v.Input})
			// 文件工具立即发 file.change；TodoWrite 发 todo.update
			events = append(events, a.deriveFromToolUse(v, ctx, ts)...)
		case types.ToolResultBlock:
			// tool.result 事件
			events = append(events, types.NewToolResult(ts, v.ToolUseId, v.Output, v.IsError))
			// Bash 衍生 command.exec
			events = append(events, a.deriveFromToolResult(v, ctx, ts)...)
			// 清理登记
			ctx.ForgetToolUse(v.ToolUseId)
		}
	}

	// message 事件（含全部 blocks）
	events = append(events, types.NewMessage(ts, role, blocks))
	return events
}

// deriveFromToolUse 在 tool_use 到达时衍生事件：
//   - Write/Edit/MultiEdit → file.change（create/edit）
//   - TodoWrite → todo.update
func (a *ClaudeCodeAdapter) deriveFromToolUse(tu types.ToolUseBlock, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var events []types.UnifiedEvent
	switch tu.ToolName {
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		// 解析 input.file_path（claude-code 字段名）
		var input struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(tu.Input, &input)
		if input.FilePath == "" {
			return nil
		}
		action := "edit"
		if tu.ToolName == "Write" {
			action = "create"
		}
		fc := types.FileChange{Path: input.FilePath, Action: action}
		events = append(events, types.NewFileChange(ts, fc, tu.ToolUseId))

	case "TodoWrite":
		// input.todos: [{content, status, activeForm}]
		var input struct {
			Todos []struct {
				Content   string `json:"content"`
				Status    string `json:"status"`
				ActiveForm string `json:"activeForm"`
			} `json:"todos"`
		}
		if err := json.Unmarshal(tu.Input, &input); err != nil {
			return nil
		}
		items := make([]types.TodoItem, 0, len(input.Todos))
		for i, t := range input.Todos {
			status := mapTodoStatus(t.Status)
			items = append(items, types.TodoItem{
				Id:      fmt.Sprintf("todo-%d", i),
				Content: t.Content,
				Status:  status,
			})
		}
		if len(items) > 0 {
			events = append(events, types.NewTodoUpdate(ts, items))
		}
	}
	return events
}

// deriveFromToolResult 在 tool_result 到达时衍生事件：
//   - Bash → command.exec（isError→exit=1, stderr；否则 exit=0, stdout）
func (a *ClaudeCodeAdapter) deriveFromToolResult(tr types.ToolResultBlock, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	info, ok := ctx.LookupToolUse(tr.ToolUseId)
	if !ok {
		return nil
	}
	if info.Name != "Bash" {
		return nil
	}
	// claude-code tool_result.output 是字符串
	var outputStr string
	if err := json.Unmarshal(tr.Output, &outputStr); err != nil {
		// 非字符串输出（罕见），用原始 JSON
		outputStr = string(tr.Output)
	}
	// 解析 Bash input.command（登记时的 input）
	var bashInput struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(info.Input, &bashInput)

	exitCode := 0
	stdout := outputStr
	stderr := ""
	if tr.IsError {
		exitCode = 1
		stdout = ""
		stderr = outputStr
	}
	return []types.UnifiedEvent{types.NewCommandExec(ts, bashInput.Command, ctx.Cwd, &exitCode, stdout, stderr, tr.ToolUseId)}
}

// ---- result 解析 ----

func (a *ClaudeCodeAdapter) parseResult(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var res struct {
		Subtype     string  `json:"subtype"`
		IsError     bool    `json:"is_error"`
		Result      string  `json:"result"`
		SessionID   string  `json:"session_id"`
		TotalCostUsd float64 `json:"total_cost_usd"`
		DurationMs  *int64  `json:"duration_ms"`
		NumTurns    *int    `json:"num_turns"`
		InputTokens *int    `json:"usage_tokens"` // 字段名可能漂移，尽力解析
		OutputTokens *int   `json:"output_tokens"`
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		return nil
	}

	var events []types.UnifiedEvent

	// 非 success → error（致命，不可恢复）
	if res.IsError {
		falseVal := false
		events = append(events, types.NewError(ts, res.Result, &falseVal, string(ctx.Cli)))
	}

	// session.end（含统计）
	stats := &types.SessionStats{}
	hasStats := false
	if res.TotalCostUsd > 0 {
		cost := res.TotalCostUsd
		stats.CostUsd = &cost
		hasStats = true
	}
	if res.DurationMs != nil {
		stats.DurationMs = res.DurationMs
		hasStats = true
	}
	if res.NumTurns != nil {
		stats.NumTurns = res.NumTurns
		hasStats = true
	}
	if res.InputTokens != nil {
		stats.InputTokens = res.InputTokens
		hasStats = true
	}
	if res.OutputTokens != nil {
		stats.OutputTokens = res.OutputTokens
		hasStats = true
	}
	if hasStats {
		events = append(events, types.NewSessionEnd(ts, stats))
	} else {
		events = append(events, types.NewSessionEnd(ts, nil))
	}
	return events
}

// ---- 辅助 ----

// mapContent 把 claude-code message.content 归一为 []ContentBlock。
//
// 注意：claude-code 原生输出用 `id`/`name`（tool_use）和 `tool_use_id`/`content`/`is_error`（tool_result），
// 与移动端 wire 格式（toolUseId/toolName/output/isError）字段名不同。
// 这里按 claude-code 原生字段名解析，再构造为 wire 格式的 ContentBlock，
// 不能直接用 types.UnmarshalContentBlock（它期望 wire 字段名）。
func mapContent(raw json.RawMessage) []types.ContentBlock {
	if len(raw) == 0 {
		return nil
	}
	// 尝试 string（简短回复）
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []types.ContentBlock{types.TextBlock{Type: "text", Text: s}}
	}
	// 尝试 []block
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw, &rawBlocks); err != nil {
		return nil
	}
	blocks := make([]types.ContentBlock, 0, len(rawBlocks))
	for _, rb := range rawBlocks {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(rb, &head); err != nil {
			continue
		}
		switch head.Type {
		case "text":
			var b struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(rb, &b)
			blocks = append(blocks, types.TextBlock{Type: "text", Text: b.Text})
		case "thinking":
			var b struct {
				Text      string `json:"text"`
				Signature string `json:"signature"`
			}
			_ = json.Unmarshal(rb, &b)
			blocks = append(blocks, types.ThinkingBlock{Type: "thinking", Text: b.Text, Signature: b.Signature})
		case "image":
			var b struct {
				MimeType string `json:"mimeType"`
				DataB64  string `json:"dataB64"`
			}
			_ = json.Unmarshal(rb, &b)
			blocks = append(blocks, types.ImageBlock{Type: "image", MimeType: b.MimeType, DataB64: b.DataB64})
		case "tool_use":
			// claude-code 原生字段：id, name, input
			var b struct {
				Id    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			_ = json.Unmarshal(rb, &b)
			input := b.Input
			if input == nil {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, types.ToolUseBlock{
				Type:      "tool_use",
				ToolUseId: b.Id,
				ToolName:  b.Name,
				Input:     input,
			})
		case "tool_result":
			// claude-code 原生字段：tool_use_id, content, is_error
			var b struct {
				ToolUseId string          `json:"tool_use_id"`
				Content   json.RawMessage `json:"content"`
				IsError   bool            `json:"is_error"`
			}
			_ = json.Unmarshal(rb, &b)
			output := b.Content
			if output == nil {
				output = json.RawMessage(`""`)
			}
			blocks = append(blocks, types.ToolResultBlock{
				Type:      "tool_result",
				ToolUseId: b.ToolUseId,
				Output:    output,
				IsError:   b.IsError,
			})
		}
	}
	return blocks
}

// mapTodoStatus 把 claude-code todo status 归一为 wire 值。
// claude-code 用 pending/in_progress/completed，与 wire 一致。
func mapTodoStatus(s string) string {
	switch s {
	case "pending", "in_progress", "completed":
		return s
	default:
		return "pending"
	}
}

// 确保未使用的 import 不报错（ssh 在 M5 runner 中使用，此处预留）
var _ = ssh.ShellQuote
