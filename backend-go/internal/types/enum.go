// Package types 定义跨层共享的枚举与 DTO 类型。
// 枚举的 wire 值必须与移动端契约逐字对齐（见 SPEC-FLUTTER-APP.md §5 / SPEC-GO-REWRITE.md §2.5）。
package types

// CliKind 标识 AI CLI 的种类。
type CliKind string

const (
	CliClaudeCode CliKind = "claude-code"
	CliCodex      CliKind = "codex"
	CliGemini     CliKind = "gemini"
	CliTrae       CliKind = "trae"
	CliOpencode   CliKind = "opencode"
	CliLingma     CliKind = "lingma"
	CliPty        CliKind = "pty"
)

// SupportedCliKinds 是支持 API Key 配置的 CLI 列表（不含 pty）。
var SupportedCliKinds = []CliKind{
	CliClaudeCode, CliCodex, CliGemini, CliTrae, CliOpencode, CliLingma,
}

// IsSupportedCliKind 判断 c 是否为支持 API Key 配置的 CLI。
func IsSupportedCliKind(c CliKind) bool {
	for _, k := range SupportedCliKinds {
		if k == c {
			return true
		}
	}
	return false
}

// SshAuthKind 标识 SSH 认证方式。
type SshAuthKind string

const (
	AuthPassword   SshAuthKind = "password"
	AuthPrivateKey SshAuthKind = "privateKey"
	AuthAgent      SshAuthKind = "agent"
)

// ServerStatus 标识服务器最近一次连通性测试结果。
type ServerStatus string

const (
	ServerStatusOk      ServerStatus = "ok"
	ServerStatusFail    ServerStatus = "fail"
	ServerStatusUnknown ServerStatus = "unknown"
)

// TaskStatus 标识任务状态。
type TaskStatus string

const (
	TaskStatusPlanned    TaskStatus = "planned"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusArchived   TaskStatus = "archived"
)

// NormalTaskStatus 把任意字符串归一为合法 TaskStatus，非法值回退 planned。
func NormalizeTaskStatus(s string) TaskStatus {
	switch TaskStatus(s) {
	case TaskStatusPlanned, TaskStatusInProgress, TaskStatusDone, TaskStatusArchived:
		return TaskStatus(s)
	}
	return TaskStatusPlanned
}

// NormalizeServerStatus 把任意字符串归一为合法 ServerStatus，非法值回退 unknown。
func NormalizeServerStatus(s string) ServerStatus {
	switch ServerStatus(s) {
	case ServerStatusOk, ServerStatusFail, ServerStatusUnknown:
		return ServerStatus(s)
	}
	return ServerStatusUnknown
}
