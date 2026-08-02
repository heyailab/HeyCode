package adapter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/heycode/backend-go/internal/types"
)

// CodexAdapter 实现 codex CLI 适配器。
//
// 模式：NDJSON 进程型。
//   - 命令：codex exec --json --full-auto --skip-git-repo-check --cd <cwd> [--model] [resume <sid>] "<prompt>"
//   - prompt 作参数，不支持 stdin 多轮，靠 codex exec resume <sid> "<prompt>" 重启
//   - 每行输出是独立 JSON，按 type 字段分发
//   - schema 漂移：item.type 或 item.item_type；assistant_message → agent_message 归一
type CodexAdapter struct{}

func (a *CodexAdapter) Kind() types.CliKind { return types.CliCodex }

// BuildStartCommand 构造 codex 启动命令。
// prompt 作命令行参数；续接时用 `resume <sid>` 子命令。
func (a *CodexAdapter) BuildStartCommand(opts BuildCommandOpts) StartCommand {
	args := []string{
		"exec",
		"--json",
		"--full-auto",
		"--skip-git-repo-check",
		"--cd", opts.Cwd,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ResumeCliSessionId != "" {
		// 续接：codex exec resume <sid> "<prompt>"
		args = append(args, "resume", opts.ResumeCliSessionId, opts.Prompt)
	} else {
		args = append(args, opts.Prompt)
	}
	return StartCommand{Command: "codex", Args: args}
}

// BuildUserInput codex 不支持 stdin 多轮，返回 ""。
func (a *CodexAdapter) BuildUserInput(prompt string) string { return "" }

// ParseLine 解析 codex NDJSON 的一行。
//
// type 分发（见 §2.6.3）：
//   - thread.started → session.init
//   - turn.started → progress + 初始化累积
//   - item.completed → 按 item 子类型分发
//   - turn.completed → flush message + streaming.done + session.end
func (a *CodexAdapter) ParseLine(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
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
	case "thread.started":
		return a.parseThreadStarted(line, ctx, ts)
	case "turn.started":
		return a.parseTurnStarted(line, ctx, ts)
	case "item.completed":
		return a.parseItemCompleted(line, ctx, ts)
	case "turn.completed":
		return a.parseTurnCompleted(line, ctx, ts)
	default:
		return nil
	}
}

// ---- thread.started → session.init ----

func (a *CodexAdapter) parseThreadStarted(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var th struct {
		ThreadID string `json:"thread_id"`
	}
	_ = json.Unmarshal([]byte(line), &th)
	// codex 的 thread_id 作为 cliSessionId 用于续接
	return []types.UnifiedEvent{types.NewSessionInit(ts, ctx.SessionId, th.ThreadID, string(ctx.Cli), ctx.Model, ctx.Cwd)}
}

// ---- turn.started → progress + 初始化累积 ----

func (a *CodexAdapter) parseTurnStarted(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	// 每回合开始：重置累积，生成新 messageId
	ctx.CurrentMessageId = fmt.Sprintf("msg-%d", ts)
	ctx.CurrentRole = "assistant"
	ctx.CurrentBlocks = nil
	ctx.currentTurnStats = &types.SessionStats{}
	step := 1
	total := 1
	return []types.UnifiedEvent{types.NewProgress(ts, &step, &total, "回合开始")}
}

// ---- item.completed → 按子类型分发 ----

func (a *CodexAdapter) parseItemCompleted(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	// schema 漂移：item.type 或 item.item_type
	var raw struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil || len(raw.Item) == 0 {
		return nil
	}

	itemType := getItemType(raw.Item)
	switch itemType {
	case "agent_message", "assistant_message": // 旧版归一
		return a.parseAgentMessage(raw.Item, ctx, ts)
	case "reasoning":
		return a.parseReasoning(raw.Item, ctx, ts)
	case "command_execution":
		return a.parseCommandExecution(raw.Item, ctx, ts)
	case "file_change":
		return a.parseFileChangeItem(raw.Item, ctx, ts)
	case "todo_list", "plan_update":
		return a.parseTodoList(raw.Item, ctx, ts)
	default:
		return nil
	}
}

// getItemType 兼容 schema 漂移：item.type 或 item.item_type；
// 旧版 assistant_message 归一为 agent_message（见 §4.13）。
func getItemType(item json.RawMessage) string {
	var t struct {
		Type     string `json:"type"`
		ItemType string `json:"item_type"`
	}
	_ = json.Unmarshal(item, &t)
	if t.Type == "" {
		return t.ItemType
	}
	if t.Type == "assistant_message" {
		return "agent_message"
	}
	return t.Type
}

// parseAgentMessage → streaming.delta（累积文本到 CurrentBlocks）
func (a *CodexAdapter) parseAgentMessage(item json.RawMessage, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var m struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(item, &m)
	if m.Text == "" {
		return nil
	}
	// 累积到 CurrentBlocks（回合结束 flush 为 message）
	ctx.CurrentBlocks = append(ctx.CurrentBlocks, types.TextBlock{Type: "text", Text: m.Text})
	return []types.UnifiedEvent{types.NewStreamingDelta(ts, ctx.CurrentMessageId, m.Text)}
}

// parseReasoning → thinking
func (a *CodexAdapter) parseReasoning(item json.RawMessage, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var r struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(item, &r)
	if r.Text == "" {
		return nil
	}
	return []types.UnifiedEvent{types.NewThinking(ts, r.Text)}
}

// parseCommandExecution → tool.use + command.exec(started)；
// 同一 item 含 exit_code 时 → tool.result + command.exec(completed)
func (a *CodexAdapter) parseCommandExecution(item json.RawMessage, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var ce struct {
		ID           string          `json:"id"`
		Command      json.RawMessage `json:"command"`     // string 或 {cmd:...}
		Args         []string        `json:"args"`
		ExitCode     *int            `json:"exit_code"`
		Output       string          `json:"output"`
		Stdout       string          `json:"stdout"`
		Stderr       string          `json:"stderr"`
	}
	_ = json.Unmarshal(item, &ce)

	// 提取命令字符串（command 可能是 string 或 {cmd:"..."})
	cmdStr := extractCommand(ce.Command)
	if len(ce.Args) > 0 {
		cmdStr = strings.Join(append([]string{cmdStr}, ce.Args...), " ")
	}

	toolUseID := ce.ID
	if toolUseID == "" {
		toolUseID = fmt.Sprintf("codex-cmd-%d", ts)
	}

	// 构造 input JSON
	inputJSON, _ := json.Marshal(map[string]string{"command": cmdStr})

	var events []types.UnifiedEvent

	// tool.use（command_execution 既是 use 又含 result）
	events = append(events, types.NewToolUse(ts, toolUseID, "Bash", inputJSON))

	if ce.ExitCode != nil {
		// 已完成：补 tool.result + command.exec(completed)
		stdout := ce.Stdout
		if stdout == "" {
			stdout = ce.Output
		}
		isError := *ce.ExitCode != 0
		outputJSON, _ := json.Marshal(stdout)
		events = append(events, types.NewToolResult(ts, toolUseID, outputJSON, isError))
		events = append(events, types.NewCommandExec(ts, cmdStr, ctx.Cwd, ce.ExitCode, stdout, ce.Stderr, toolUseID))
	} else {
		// 仅 started（无 exit_code）
		events = append(events, types.NewCommandExec(ts, cmdStr, ctx.Cwd, nil, "", "", toolUseID))
	}
	return events
}

// parseFileChangeItem → 遍历 changes 发多个 file.change
func (a *CodexAdapter) parseFileChangeItem(item json.RawMessage, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var fc struct {
		ID      string `json:"id"`
		Changes []struct {
			Path    string `json:"path"`
			Action  string `json:"action"` // create | edit | delete
			Diff    string `json:"diff"`
			Added   *int   `json:"added_lines"`
			Removed *int   `json:"removed_lines"`
		} `json:"changes"`
	}
	_ = json.Unmarshal(item, &fc)

	toolUseID := fc.ID
	if toolUseID == "" {
		toolUseID = fmt.Sprintf("codex-file-%d", ts)
	}

	var events []types.UnifiedEvent
	for _, c := range fc.Changes {
		change := types.FileChange{
			Path:         c.Path,
			Action:       normalizeFileAction(c.Action),
			Diff:         c.Diff,
			AddedLines:   c.Added,
			RemovedLines: c.Removed,
		}
		events = append(events, types.NewFileChange(ts, change, toolUseID))
	}
	return events
}

// parseTodoList → todo.update
func (a *CodexAdapter) parseTodoList(item json.RawMessage, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var tl struct {
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	_ = json.Unmarshal(item, &tl)
	if len(tl.Todos) == 0 {
		return nil
	}
	items := make([]types.TodoItem, 0, len(tl.Todos))
	for i, t := range tl.Todos {
		items = append(items, types.TodoItem{
			Id:      fmt.Sprintf("todo-%d", i),
			Content: t.Content,
			Status:  mapTodoStatus(t.Status),
		})
	}
	return []types.UnifiedEvent{types.NewTodoUpdate(ts, items)}
}

// ---- turn.completed → flush message + streaming.done + session.end ----

func (a *CodexAdapter) parseTurnCompleted(line string, ctx *ParseContext, ts int64) []types.UnifiedEvent {
	var events []types.UnifiedEvent

	// flush message（累积的 blocks）
	if len(ctx.CurrentBlocks) > 0 {
		events = append(events, types.NewMessage(ts, ctx.CurrentRole, ctx.CurrentBlocks))
	}
	// streaming.done
	if ctx.CurrentMessageId != "" {
		events = append(events, types.NewStreamingDone(ts, ctx.CurrentMessageId))
	}
	// session.end（含 turn 统计）
	var tc struct {
		Usage struct {
			InputTokens  *int     `json:"input_tokens"`
			OutputTokens *int     `json:"output_tokens"`
			CostUsd      *float64 `json:"cost_usd"`
		} `json:"usage"`
		DurationMs *int64 `json:"duration_ms"`
		NumTurns   *int   `json:"num_turns"`
	}
	_ = json.Unmarshal([]byte(line), &tc)

	stats := &types.SessionStats{}
	hasStats := false
	if tc.Usage.InputTokens != nil {
		stats.InputTokens = tc.Usage.InputTokens
		hasStats = true
	}
	if tc.Usage.OutputTokens != nil {
		stats.OutputTokens = tc.Usage.OutputTokens
		hasStats = true
	}
	if tc.Usage.CostUsd != nil {
		stats.CostUsd = tc.Usage.CostUsd
		hasStats = true
	}
	if tc.DurationMs != nil {
		stats.DurationMs = tc.DurationMs
		hasStats = true
	}
	if tc.NumTurns != nil {
		stats.NumTurns = tc.NumTurns
		hasStats = true
	}
	if hasStats {
		events = append(events, types.NewSessionEnd(ts, stats))
	} else {
		events = append(events, types.NewSessionEnd(ts, nil))
	}

	// 清空累积
	ctx.CurrentBlocks = nil
	ctx.CurrentMessageId = ""
	return events
}

// ---- 辅助 ----

// extractCommand 从 command 字段提取命令字符串。
// codex 的 command 可能是 string 或 {cmd:"..."} 形式。
func extractCommand(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Cmd string `json:"cmd"`
	}
	_ = json.Unmarshal(raw, &obj)
	return obj.Cmd
}

// normalizeFileAction 归一文件操作类型。
func normalizeFileAction(s string) string {
	switch s {
	case "create", "edit", "delete":
		return s
	case "write":
		return "create"
	case "modify", "update":
		return "edit"
	case "remove":
		return "delete"
	default:
		return "edit"
	}
}
