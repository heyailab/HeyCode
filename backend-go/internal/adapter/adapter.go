// Package adapter 实现 CLI 适配器，把不同 AI CLI 的输出归一为 UnifiedEvent 流。
//
// 协议见 SPEC-GO-REWRITE.md §2.6。适配器职责：
//   - BuildStartCommand：构造启动命令（command + args + env）
//   - ParseLine：解析 CLI stdout 的一行，产出 0..N 个 UnifiedEvent
//   - BuildUserInput：把用户 prompt 转为 CLI 能接受的输入（stdin 型返回 NDJSON，重启型返回 ""）
//
// 适配器是无状态的（ParseContext 由 runner per-session 维护并传入），
// 因此工厂返回的实例可被多个会话并发复用。
package adapter

import (
	"fmt"

	"github.com/heycode/backend-go/internal/types"
)

// Adapter 是 CLI 适配器的统一接口。
type Adapter interface {
	// Kind 返回适配器对应的 CliKind。
	Kind() types.CliKind

	// BuildStartCommand 构造启动 CLI 进程的命令。
	// opts 含 cwd/prompt/model/resumeCliSessionId/allowedTools。
	// 返回的 StartCommand 由 runner 传给 ssh.SpawnStream。
	BuildStartCommand(opts BuildCommandOpts) StartCommand

	// ParseLine 解析 CLI stdout 的一行，返回 0..N 个事件。
	// ctx 是 per-session 的解析状态（pendingToolUseIds 等），可读写。
	// timestamp 由 runner 提供（保证同一行的事件时间戳一致）。
	ParseLine(line string, ctx *ParseContext, timestamp int64) []types.UnifiedEvent

	// BuildUserInput 把用户 prompt 转为写入 CLI stdin 的字符串。
	// 不支持 stdin 多轮的适配器返回 ""。
	BuildUserInput(prompt string) string
}

// TimelineGenerator 是可选接口。
// 适配器实现该接口时，runner 走 Mock 路径：不连 SSH，
// 用 25ms delay 逐行把 GenerateTimeline 的输出喂给 ParseLine。
type TimelineGenerator interface {
	GenerateTimeline(prompt string) []string
}

// BuildCommandOpts 是 BuildStartCommand 的入参。
type BuildCommandOpts struct {
	Cwd                string
	Prompt             string
	Model              string
	ResumeCliSessionId string   // 续接时非空
	AllowedTools       []string // 可选，限制工具集
}

// StartCommand 是 BuildStartCommand 的返回。
// Command 是可执行文件名（如 "claude"），Args 是参数列表（已 ShellQuote）。
// Env 是环境变量（KEY=VALUE），由 runner 通过 SpawnOptions.Env 传递。
type StartCommand struct {
	Command string
	Args    []string
	Env     map[string]string
}

// ParseContext 是 per-session 的解析状态，由 runner 创建并传入 ParseLine。
//
// 字段语义：
//   - SessionId/Cwd/Cli/Model：会话基本信息，构造 session.init 等事件用
//   - PendingToolUseIds：FIFO 队列，tool_use 入队，tool_result 出队匹配
//     （opencode 的 tool_finish 用 FIFO shift；claude-code 用精确 id 匹配）
//   - ToolUseIndex：toolUseId → {Name, Input}，tool_result 到达时识别特殊工具
//     （Bash→command.exec，Write/Edit→file.change 等）
//   - CurrentMessageId/CurrentRole/CurrentBlocks：流式适配器累积片段，
//     回合结束 flush 为 message + streaming.done
type ParseContext struct {
	SessionId         string
	Cwd               string
	Cli               types.CliKind
	Model             string
	PendingToolUseIds []string
	ToolUseIndex      map[string]ToolUseInfo
	CurrentMessageId  string
	CurrentRole       string
	CurrentBlocks     []types.ContentBlock

	// sessionInitSent 防止重复发 session.init（codex/opencode 首行发一次）
	sessionInitSent bool

	// currentTurnStats 流式累积 turn 统计（codex/opencode 用）
	currentTurnStats *types.SessionStats
}

// ToolUseInfo 是 ToolUseIndex 的值，记录某次 tool_use 的元信息。
type ToolUseInfo struct {
	Name  string
	Input []byte // 原始 JSON input
}

// NewParseContext 创建 per-session 解析状态。
func NewParseContext(sessionID, cwd string, cli types.CliKind, model string) *ParseContext {
	return &ParseContext{
		SessionId:    sessionID,
		Cwd:          cwd,
		Cli:          cli,
		Model:        model,
		ToolUseIndex: make(map[string]ToolUseInfo),
	}
}

// EnqueueToolUse 把 toolUseId 入队并登记到 ToolUseIndex。
func (c *ParseContext) EnqueueToolUse(id string, info ToolUseInfo) {
	c.PendingToolUseIds = append(c.PendingToolUseIds, id)
	c.ToolUseIndex[id] = info
}

// ShiftToolUse FIFO 出队，返回队首 id（opencode tool_finish 用）。
// 队列空返回 ""。
func (c *ParseContext) ShiftToolUse() string {
	if len(c.PendingToolUseIds) == 0 {
		return ""
	}
	id := c.PendingToolUseIds[0]
	c.PendingToolUseIds = c.PendingToolUseIds[1:]
	return id
}

// LookupToolUse 按 id 查找（claude-code 精确匹配用）。
func (c *ParseContext) LookupToolUse(id string) (ToolUseInfo, bool) {
	info, ok := c.ToolUseIndex[id]
	return info, ok
}

// ForgetToolUse 移除某 id（tool_result 处理完后清理）。
func (c *ParseContext) ForgetToolUse(id string) {
	delete(c.ToolUseIndex, id)
	// 也从 FIFO 队列移除（若存在）
	for i, x := range c.PendingToolUseIds {
		if x == id {
			c.PendingToolUseIds = append(c.PendingToolUseIds[:i], c.PendingToolUseIds[i+1:]...)
			break
		}
	}
}

// ---- 工厂 ----

// Get 返回指定 CliKind 的适配器实例。
// gemini/lingma 无专用适配器，返回 error 引导用 pty 兜底（见 §2.6.3）。
// 返回的实例是无状态的，可被多会话并发复用。
func Get(kind types.CliKind) (Adapter, error) {
	switch kind {
	case types.CliClaudeCode:
		return &ClaudeCodeAdapter{}, nil
	case types.CliTrae:
		return &TraeAdapter{}, nil
	case types.CliCodex:
		return &CodexAdapter{}, nil
	case types.CliOpencode:
		return &OpencodeAdapter{}, nil
	case types.CliPty:
		return &PtyAdapter{}, nil
	case types.CliGemini, types.CliLingma:
		// 无专用适配器，引导用 pty 兜底
		return nil, fmt.Errorf("%s 暂不支持专用适配器，请使用 pty 降级模式", kind)
	default:
		return nil, fmt.Errorf("unknown cli kind: %s", kind)
	}
}
