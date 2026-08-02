// Package types 定义统一事件类型（UnifiedEvent）与 ContentBlock。
//
// 事件协议见 SPEC-GO-REWRITE.md §2.5 / SPEC-FLUTTER-APP.md 附录 C。
// 约束：
//   - 所有事件共享 timestamp（毫秒 epoch）和 type（字符串）
//   - wire 值必须与移动端逐字对齐，不得发明新 type
//   - 未知 type 在 App 端会被包装成 ErrorEvent
//
// Go 实现采用「interface + 具体结构体」的判别联合：
//   - UnifiedEvent 是空接口，每个具体事件实现 EventType() string
//   - 每个具体事件 struct 内嵌 EventBase（含 Type/Timestamp）+ 自身字段
//   - json.Marshal(interface 持有具体值) 会按具体类型字段输出，无需自定义 marshaler
//   - json.Unmarshal 需先读 type 再分发，由 UnmarshalEvent 提供
package types

import (
	"encoding/json"
	"fmt"
)

// EventType 是事件的 type 字段（wire 字符串）。
type EventType string

// 13 种事件 wire 值（必须逐字对齐 §2.5.3）。
const (
	EventSessionInit    EventType = "session.init"
	EventMessage        EventType = "message"
	EventStreamingDelta EventType = "streaming.delta"
	EventStreamingDone  EventType = "streaming.done"
	EventToolUse        EventType = "tool.use"
	EventToolResult     EventType = "tool.result"
	EventFileChange     EventType = "file.change"
	EventCommandExec    EventType = "command.exec"
	EventTodoUpdate     EventType = "todo.update"
	EventThinking       EventType = "thinking"
	EventProgress       EventType = "progress"
	EventError          EventType = "error"
	EventSessionEnd     EventType = "session.end"
)

// UnifiedEvent 是所有事件的统一接口。
// M4 适配器 ParseLine 返回 []UnifiedEvent；M5 eventbus 持久化/回放也用它。
type UnifiedEvent interface {
	EventType() string
}

// EventBase 是所有事件共享的字段。具体事件 struct 内嵌它。
//
// Type 字段在构造时由 NewXxx 辅助函数自动填入，避免手写出错。
type EventBase struct {
	Type      EventType `json:"type"`
	Timestamp int64     `json:"timestamp"` // 毫秒 epoch
}

// ---- 13 种具体事件 ----

// SessionInitEvent CLI 进程就绪。
type SessionInitEvent struct {
	EventBase
	SessionId    string `json:"sessionId"`
	CliSessionId string `json:"cliSessionId,omitempty"`
	Cli          string `json:"cli"`
	Model        string `json:"model,omitempty"`
	Cwd          string `json:"cwd"`
}

func (e SessionInitEvent) EventType() string { return string(e.Type) }

// MessageEvent 一条完整消息（user/assistant）。
type MessageEvent struct {
	EventBase
	Role   string         `json:"role"` // "user" | "assistant"
	Blocks []ContentBlock `json:"blocks"`
}

func (e MessageEvent) EventType() string { return string(e.Type) }

// UnmarshalJSON 自定义反序列化：blocks 是 ContentBlock interface 切片，
// 标准 json.Unmarshal 无法推断具体类型，需逐元素用 UnmarshalContentBlock 分发。
func (e *MessageEvent) UnmarshalJSON(data []byte) error {
	var aux struct {
		Type      string            `json:"type"`
		Timestamp int64             `json:"timestamp"`
		Role      string            `json:"role"`
		Blocks    []json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.Type = EventType(aux.Type)
	e.Timestamp = aux.Timestamp
	e.Role = aux.Role
	e.Blocks = make([]ContentBlock, 0, len(aux.Blocks))
	for _, rb := range aux.Blocks {
		b, err := UnmarshalContentBlock(rb)
		if err != nil {
			// 跳过无法识别的块（前向兼容）
			continue
		}
		e.Blocks = append(e.Blocks, b)
	}
	return nil
}

// StreamingDeltaEvent 流式文本增量。
type StreamingDeltaEvent struct {
	EventBase
	MessageId  string `json:"messageId"`
	TextDelta  string `json:"textDelta,omitempty"`
}

func (e StreamingDeltaEvent) EventType() string { return string(e.Type) }

// StreamingDoneEvent 流式消息结束。
type StreamingDoneEvent struct {
	EventBase
	MessageId string `json:"messageId"`
}

func (e StreamingDoneEvent) EventType() string { return string(e.Type) }

// ToolUseEvent 工具调用开始。
type ToolUseEvent struct {
	EventBase
	ToolUseId string          `json:"toolUseId"`
	ToolName  string          `json:"toolName"`
	Input     json.RawMessage `json:"input"` // 任意 JSON
}

func (e ToolUseEvent) EventType() string { return string(e.Type) }

// ToolResultEvent 工具返回。
type ToolResultEvent struct {
	EventBase
	ToolUseId string          `json:"toolUseId"`
	Output    json.RawMessage `json:"output"` // string | {type:"json",json} | {type:"image",dataB64}
	IsError   bool            `json:"isError,omitempty"`
}

func (e ToolResultEvent) EventType() string { return string(e.Type) }

// FileChangeEvent 文件增删改。
type FileChangeEvent struct {
	EventBase
	Change    FileChange `json:"change"`
	ToolUseId string     `json:"toolUseId,omitempty"`
}

func (e FileChangeEvent) EventType() string { return string(e.Type) }

// CommandExecEvent shell 命令执行。
type CommandExecEvent struct {
	EventBase
	Command  string `json:"command"`
	Cwd      string `json:"cwd,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"` // nil=未结束/无退出码
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ToolUseId string `json:"toolUseId,omitempty"`
}

func (e CommandExecEvent) EventType() string { return string(e.Type) }

// TodoUpdateEvent TodoList 变更（整体替换）。
type TodoUpdateEvent struct {
	EventBase
	Todos []TodoItem `json:"todos"`
}

func (e TodoUpdateEvent) EventType() string { return string(e.Type) }

// ThinkingEvent 模型思考。
type ThinkingEvent struct {
	EventBase
	Text string `json:"text"`
}

func (e ThinkingEvent) EventType() string { return string(e.Type) }

// ProgressEvent 步骤进度。
type ProgressEvent struct {
	EventBase
	Step    *int   `json:"step,omitempty"`
	Total   *int   `json:"total,omitempty"`
	Message string `json:"message,omitempty"`
}

func (e ProgressEvent) EventType() string { return string(e.Type) }

// ErrorEvent 出错；recoverable=false 致命。
type ErrorEvent struct {
	EventBase
	Message     string `json:"message"`
	Recoverable *bool  `json:"recoverable,omitempty"`
	Cli         string `json:"cli,omitempty"`
}

func (e ErrorEvent) EventType() string { return string(e.Type) }

// SessionEndEvent 会话结束。
type SessionEndEvent struct {
	EventBase
	Stats *SessionStats `json:"stats,omitempty"`
}

func (e SessionEndEvent) EventType() string { return string(e.Type) }

// SessionStats 是 session.end 的统计信息（全可选）。
type SessionStats struct {
	CostUsd       *float64 `json:"costUsd,omitempty"`
	DurationMs    *int64   `json:"durationMs,omitempty"`
	NumTurns      *int     `json:"numTurns,omitempty"`
	InputTokens  *int     `json:"inputTokens,omitempty"`
	OutputTokens *int     `json:"outputTokens,omitempty"`
}

// ---- ContentBlock（message.blocks 元素，5 种）----

// ContentBlock 是消息块元素的判别联合（§2.5.2）。
// 用 interface + 具体类型，json.Marshal 按具体类型输出。
type ContentBlock interface {
	blockType() string
}

// TextBlock 纯文本。
type TextBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

func (TextBlock) blockType() string { return "text" }

// ThinkingBlock 模型思考（带可选签名）。
type ThinkingBlock struct {
	Type      string `json:"type"` // "thinking"
	Text      string `json:"text"`
	Signature string `json:"signature,omitempty"`
}

func (ThinkingBlock) blockType() string { return "thinking" }

// ImageBlock 图片。
type ImageBlock struct {
	Type     string `json:"type"` // "image"
	MimeType string `json:"mimeType"`
	DataB64  string `json:"dataB64"`
}

func (ImageBlock) blockType() string { return "image" }

// ToolUseBlock 工具调用（在 message 内的表示）。
type ToolUseBlock struct {
	Type      string          `json:"type"` // "tool_use"
	ToolUseId string          `json:"toolUseId"`
	ToolName  string          `json:"toolName"`
	Input     json.RawMessage `json:"input"`
}

func (ToolUseBlock) blockType() string { return "tool_use" }

// ToolResultBlock 工具结果。
// Output 是 string | {type:"json",json} | {type:"image",dataB64}，
// 用 json.RawMessage 保留原样，由适配器决定具体形态。
type ToolResultBlock struct {
	Type      string          `json:"type"` // "tool_result"
	ToolUseId string          `json:"toolUseId"`
	Output    json.RawMessage `json:"output"`
	IsError   bool            `json:"isError,omitempty"`
}

func (ToolResultBlock) blockType() string { return "tool_result" }

// ---- FileChange / TodoItem ----

// FileChange 是文件变更的描述（file.change 事件的 change 字段）。
type FileChange struct {
	Path         string `json:"path"` // 绝对路径
	Action       string `json:"action"` // create | edit | delete
	Diff         string `json:"diff,omitempty"`
	AddedLines   *int   `json:"addedLines,omitempty"`
	RemovedLines *int   `json:"removedLines,omitempty"`
}

// TodoItem 是 TodoList 的单项（todo.update 事件的 todos 元素）。
type TodoItem struct {
	Id       string `json:"id"` // 会话内稳定
	Content  string `json:"content"`
	Status   string `json:"status"` // pending | in_progress | completed
	Progress *int   `json:"progress,omitempty"`
}

// ---- 构造辅助函数（自动填 Type，减少手写出错）----

// NewSessionInit 构造 session.init 事件。
func NewSessionInit(ts int64, sessionID, cliSessionID, cli, model, cwd string) SessionInitEvent {
	return SessionInitEvent{
		EventBase:    EventBase{Type: EventSessionInit, Timestamp: ts},
		SessionId:    sessionID,
		CliSessionId: cliSessionID,
		Cli:          cli,
		Model:        model,
		Cwd:          cwd,
	}
}

// NewMessage 构造 message 事件。
func NewMessage(ts int64, role string, blocks []ContentBlock) MessageEvent {
	return MessageEvent{
		EventBase: EventBase{Type: EventMessage, Timestamp: ts},
		Role:      role,
		Blocks:    blocks,
	}
}

// NewStreamingDelta 构造 streaming.delta 事件。
func NewStreamingDelta(ts int64, messageID, textDelta string) StreamingDeltaEvent {
	return StreamingDeltaEvent{
		EventBase: EventBase{Type: EventStreamingDelta, Timestamp: ts},
		MessageId: messageID,
		TextDelta: textDelta,
	}
}

// NewStreamingDone 构造 streaming.done 事件。
func NewStreamingDone(ts int64, messageID string) StreamingDoneEvent {
	return StreamingDoneEvent{
		EventBase: EventBase{Type: EventStreamingDone, Timestamp: ts},
		MessageId: messageID,
	}
}

// NewToolUse 构造 tool.use 事件。input 为 nil 时输出 {}。
func NewToolUse(ts int64, toolUseID, toolName string, input json.RawMessage) ToolUseEvent {
	if input == nil {
		input = json.RawMessage("{}")
	}
	return ToolUseEvent{
		EventBase: EventBase{Type: EventToolUse, Timestamp: ts},
		ToolUseId: toolUseID,
		ToolName:  toolName,
		Input:     input,
	}
}

// NewToolResult 构造 tool.result 事件。output 为 nil 时输出 ""。
func NewToolResult(ts int64, toolUseID string, output json.RawMessage, isError bool) ToolResultEvent {
	if output == nil {
		output = json.RawMessage(`""`)
	}
	return ToolResultEvent{
		EventBase: EventBase{Type: EventToolResult, Timestamp: ts},
		ToolUseId: toolUseID,
		Output:    output,
		IsError:   isError,
	}
}

// NewFileChange 构造 file.change 事件。
func NewFileChange(ts int64, change FileChange, toolUseID string) FileChangeEvent {
	return FileChangeEvent{
		EventBase: EventBase{Type: EventFileChange, Timestamp: ts},
		Change:    change,
		ToolUseId: toolUseID,
	}
}

// NewCommandExec 构造 command.exec 事件。exitCode 为 nil 表示未结束。
func NewCommandExec(ts int64, command, cwd string, exitCode *int, stdout, stderr, toolUseID string) CommandExecEvent {
	return CommandExecEvent{
		EventBase: EventBase{Type: EventCommandExec, Timestamp: ts},
		Command:   command,
		Cwd:       cwd,
		ExitCode:  exitCode,
		Stdout:    stdout,
		Stderr:    stderr,
		ToolUseId: toolUseID,
	}
}

// NewTodoUpdate 构造 todo.update 事件。
func NewTodoUpdate(ts int64, todos []TodoItem) TodoUpdateEvent {
	return TodoUpdateEvent{
		EventBase: EventBase{Type: EventTodoUpdate, Timestamp: ts},
		Todos:     todos,
	}
}

// NewThinking 构造 thinking 事件。
func NewThinking(ts int64, text string) ThinkingEvent {
	return ThinkingEvent{
		EventBase: EventBase{Type: EventThinking, Timestamp: ts},
		Text:      text,
	}
}

// NewProgress 构造 progress 事件。
func NewProgress(ts int64, step, total *int, message string) ProgressEvent {
	return ProgressEvent{
		EventBase: EventBase{Type: EventProgress, Timestamp: ts},
		Step:      step,
		Total:     total,
		Message:   message,
	}
}

// NewError 构造 error 事件。recoverable 为 nil 时省略。
func NewError(ts int64, message string, recoverable *bool, cli string) ErrorEvent {
	return ErrorEvent{
		EventBase:  EventBase{Type: EventError, Timestamp: ts},
		Message:    message,
		Recoverable: recoverable,
		Cli:        cli,
	}
}

// NewSessionEnd 构造 session.end 事件。
func NewSessionEnd(ts int64, stats *SessionStats) SessionEndEvent {
	return SessionEndEvent{
		EventBase: EventBase{Type: EventSessionEnd, Timestamp: ts},
		Stats:     stats,
	}
}

// ---- Marshal / Unmarshal ----

// MarshalEvent 把 UnifiedEvent 序列化为 JSON 字节。
// interface 持有具体类型时，json.Marshal 按具体类型字段输出。
func MarshalEvent(e UnifiedEvent) ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalEvent 先读 type 字段再分发到具体事件类型。
// 未知 type 返回错误（App 端会包装成 ErrorEvent，后端解析失败应告警而非吞掉）。
func UnmarshalEvent(data []byte) (UnifiedEvent, error) {
	// 先解析出 type
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("unmarshal event type: %w", err)
	}

	switch EventType(head.Type) {
	case EventSessionInit:
		var e SessionInitEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventMessage:
		var e MessageEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventStreamingDelta:
		var e StreamingDeltaEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventStreamingDone:
		var e StreamingDoneEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventToolUse:
		var e ToolUseEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventToolResult:
		var e ToolResultEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventFileChange:
		var e FileChangeEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventCommandExec:
		var e CommandExecEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventTodoUpdate:
		var e TodoUpdateEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventThinking:
		var e ThinkingEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventProgress:
		var e ProgressEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventError:
		var e ErrorEvent
		err := json.Unmarshal(data, &e)
		return e, err
	case EventSessionEnd:
		var e SessionEndEvent
		err := json.Unmarshal(data, &e)
		return e, err
	default:
		return nil, fmt.Errorf("unknown event type: %s", head.Type)
	}
}

// ---- ContentBlock Unmarshal（message.blocks 元素需要按 type 分发）----

// UnmarshalContentBlock 先读 type 字段再分发到具体 ContentBlock 类型。
func UnmarshalContentBlock(data []byte) (ContentBlock, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("unmarshal content block type: %w", err)
	}
	switch head.Type {
	case "text":
		var b TextBlock
		err := json.Unmarshal(data, &b)
		return b, err
	case "thinking":
		var b ThinkingBlock
		err := json.Unmarshal(data, &b)
		return b, err
	case "image":
		var b ImageBlock
		err := json.Unmarshal(data, &b)
		return b, err
	case "tool_use":
		var b ToolUseBlock
		err := json.Unmarshal(data, &b)
		return b, err
	case "tool_result":
		var b ToolResultBlock
		err := json.Unmarshal(data, &b)
		return b, err
	default:
		return nil, fmt.Errorf("unknown content block type: %s", head.Type)
	}
}
