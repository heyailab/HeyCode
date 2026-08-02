package adapter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/heycode/backend-go/internal/types"
)

// OpencodeAdapter 实现 opencode CLI 适配器。
//
// 模式：NDJSON 进程型。
//   - 命令：opencode run --format json --dangerously-skip-permissions --cwd <cwd> [--model] [--continue <sid>] "<prompt>"
//   - prompt 作参数，不支持 stdin，靠 --continue <sid> 重启
//   - tool_finish 用 FIFO shift 匹配 toolUseId（无精确 id 关联）
//   - 回合结束 flush message + streaming.done + session.end
type OpencodeAdapter struct{}

func (a *OpencodeAdapter) Kind() types.CliKind { return types.CliOpencode }

// BuildStartCommand 构造 opencode 启动命令。
func (a *OpencodeAdapter) BuildStartCommand(opts BuildCommandOpts) StartCommand {
	args := []string{
		"run",
		"--format", "json",
		"--dangerously-skip-permissions",
		"--cwd", opts.Cwd,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ResumeCliSessionId != "" {
		args = append(args, "--continue", opts.ResumeCliSessionId)
	}
	args = append(args, opts.Prompt)
	return StartCommand{Command: "opencode", Args: args}
}

// BuildUserInput opencode 不支持 stdin，返回 ""。
func (a *OpencodeAdapter) BuildUserInput(prompt string) string { return "" }

// ParseLine 解析 opencode NDJSON 的一行。
//
// type 分发（见 §2.6.3）：
//   - step_start → 首次发 session.init + progress
//   - text → streaming.delta 累积
//   - reasoning → thinking
//   - tool_start → tool.use + write/edit→file.change + bash→command.exec(started)
//   - tool_finish → FIFO shift 匹配 + tool.result + bash 补 command.exec(completed)
//   - step_finish → flush + streaming.done + session.end
func (a *OpencodeAdapter) ParseLine(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &head); err != nil {
		return nil
	}

	switch head.Type {
	case "step_start":
		return a.parseStepStart(line, ctx, ts)
	case "text":
		return a.parseText(line, ctx, ts)
	case "reasoning":
		return a.parseReasoning(line, ctx, ts)
	case "tool_start":
		return a.parseToolStart(line, ctx, ts)
	case "tool_finish":
		return a.parseToolFinish(line, ctx, ts)
	case "step_finish":
		return a.parseStepFinish(line, ctx, ts)
	default:
		return nil
	}
}

// ---- step_start → 首次 session.init + progress ----

func (a *OpencodeAdapter) parseStepStart(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	// 每步开始：重置累积
	ctx.CurrentMessageId = fmt.Sprintf("msg-%d", ts)
	ctx.CurrentRole = "assistant"
	ctx.CurrentBlocks = nil

	var events []types.UnifiedEvent
	// 首次 step 发 session.init
	if !ctx.sessionInitSent {
		ctx.sessionInitSent = true
		events = append(events, types.NewSessionInit(ts, ctx.SessionId, "", string(ctx.Cli), ctx.Model, ctx.Cwd))
	}
	step := 1
	events = append(events, types.NewProgress(ts, &step, nil, "步骤开始"))
	return events
}

// ---- text → streaming.delta 累积 ----

func (a *OpencodeAdapter) parseText(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var t struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(line), &t)
	if t.Text == "" {
		return nil
	}
	ctx.CurrentBlocks = append(ctx.CurrentBlocks, types.TextBlock{Type: "text", Text: t.Text})
	return []types.UnifiedEvent{types.NewStreamingDelta(ts, ctx.CurrentMessageId, t.Text)}
}

// ---- reasoning → thinking ----

func (a *OpencodeAdapter) parseReasoning(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var r struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(line), &r)
	if r.Text == "" {
		return nil
	}
	return []types.UnifiedEvent{types.NewThinking(ts, r.Text)}
}

// ---- tool_start → tool.use + 衍生（write/edit→file.change, bash→command.exec started）----

func (a *OpencodeAdapter) parseToolStart(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var ts2 struct {
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	_ = json.Unmarshal([]byte(line), &ts2)
	if ts2.ID == "" {
		ts2.ID = fmt.Sprintf("oc-tool-%d", ts)
	}
	if ts2.Input == nil {
		ts2.Input = json.RawMessage("{}")
	}

	var events []types.UnifiedEvent
	// tool.use
	events = append(events, types.NewToolUse(ts, ts2.ID, ts2.Name, ts2.Input))
	// 入队（tool_finish 用 FIFO shift 匹配）
	ctx.EnqueueToolUse(ts2.ID, ToolUseInfo{Name: ts2.Name, Input: ts2.Input})

	// 衍生：write/edit → file.change；bash → command.exec(started)
	switch ts2.Name {
	case "write", "edit":
		events = append(events, a.deriveFileChangeFromTool(ts2.ID, ts2.Name, ts2.Input, ctx, ts)...)
	case "bash":
		events = append(events, a.deriveCommandStartFromBash(ts2.ID, ts2.Input, ctx, ts)...)
	}
	return events
}

// ---- tool_finish → FIFO shift + tool.result + bash 补 command.exec(completed)----

func (a *OpencodeAdapter) parseToolFinish(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var tf struct {
		Output    json.RawMessage `json:"output"`
		Error     string          `json:"error"`
		ExitCode  *int            `json:"exit_code"`
	}
	_ = json.Unmarshal([]byte(line), &tf)

	// FIFO shift 匹配 toolUseId
	toolUseID := ctx.ShiftToolUse()
	if toolUseID == "" {
		return nil
	}
	info, ok := ctx.LookupToolUse(toolUseID)
	if !ok {
		return nil
	}

	isError := tf.Error != ""
	var output json.RawMessage
	if isError {
		output, _ = json.Marshal(tf.Error)
	} else if len(tf.Output) > 0 {
		output = tf.Output
	} else {
		output = json.RawMessage(`""`)
	}

	var events []types.UnifiedEvent
	// tool.result
	events = append(events, types.NewToolResult(ts, toolUseID, output, isError))

	// bash 补 command.exec(completed)
	if info.Name == "bash" {
		var bashInput struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(info.Input, &bashInput)
		exitCode := tf.ExitCode
		if exitCode == nil {
			if isError {
				code := 1
				exitCode = &code
			} else {
				code := 0
				exitCode = &code
			}
		}
		var stdout, stderr string
		if isError {
			stderr = tf.Error
		} else {
			stdout = rawToString(tf.Output)
		}
		events = append(events, types.NewCommandExec(ts, bashInput.Command, ctx.Cwd, exitCode, stdout, stderr, toolUseID))
	}

	// 清理登记
	ctx.ForgetToolUse(toolUseID)
	return events
}

// ---- step_finish → flush + streaming.done + session.end ----

func (a *OpencodeAdapter) parseStepFinish(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var events []types.UnifiedEvent
	// flush message
	if len(ctx.CurrentBlocks) > 0 {
		events = append(events, types.NewMessage(ts, ctx.CurrentRole, ctx.CurrentBlocks))
	}
	// streaming.done
	if ctx.CurrentMessageId != "" {
		events = append(events, types.NewStreamingDone(ts, ctx.CurrentMessageId))
	}
	// session.end（opencode step_finish 通常不含统计，发空 stats）
	events = append(events, types.NewSessionEnd(ts, nil))

	// 清空累积
	ctx.CurrentBlocks = nil
	ctx.CurrentMessageId = ""
	return events
}

// ---- 衍生辅助 ----

// deriveFileChangeFromTool 从 write/edit 工具 input 提取文件路径发 file.change。
func (a *OpencodeAdapter) deriveFileChangeFromTool(toolUseID, toolName string, input json.RawMessage, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var in struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	}
	_ = json.Unmarshal(input, &in)
	path := in.Path
	if path == "" {
		path = in.FilePath
	}
	if path == "" {
		return nil
	}
	action := "edit"
	if toolName == "write" {
		action = "create"
	}
	return []types.UnifiedEvent{types.NewFileChange(ts, types.FileChange{Path: path, Action: action}, toolUseID)}
}

// deriveCommandStartFromBash 从 bash 工具 input 提取命令发 command.exec(started)。
func (a *OpencodeAdapter) deriveCommandStartFromBash(toolUseID string, input json.RawMessage, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var in struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(input, &in)
	if in.Command == "" {
		return nil
	}
	return []types.UnifiedEvent{types.NewCommandExec(ts, in.Command, ctx.Cwd, nil, "", "", toolUseID)}
}

// rawToString 把 json.RawMessage 转为字符串（如果是 JSON 字符串）。
func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
