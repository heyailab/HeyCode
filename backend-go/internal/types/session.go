package types

import "time"

// SessionStatus 标识会话状态（见 §2.5.1）。
//
// 注意：本类型与 store.SessionStatus 是同一概念；
// 这里用 wire 层类型避免 store 包导出 SessionStatus 后 types 依赖 store。
// 实际 wire 值由 store 层 Normalize 时映射为本类型字符串。
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusIdle    SessionStatus = "idle"
	SessionStatusEnded   SessionStatus = "ended"
	SessionStatusError   SessionStatus = "error"
)

// NormalizeSessionStatus 把任意字符串归一为合法 SessionStatus，非法值回退 idle。
func NormalizeSessionStatus(s string) SessionStatus {
	switch SessionStatus(s) {
	case SessionStatusRunning, SessionStatusIdle, SessionStatusEnded, SessionStatusError:
		return SessionStatus(s)
	}
	return SessionStatusIdle
}

// SessionDTO 是 Session 的对外响应 DTO（见 §2.3.6）。
//
// TaskID 可空（WS 直接启动会话时为 null）；
// CliSessionID 可空（收到 session.init 后才写入）；
// EndedAt 可空（status=running/idle 时为 null）；
// Model 可空。
type SessionDTO struct {
	ID           string     `json:"id"`
	TaskID       *string    `json:"taskId,omitempty"`
	CliSessionID string     `json:"cliSessionId,omitempty"`
	Cli          CliKind    `json:"cli"`
	Model        string     `json:"model,omitempty"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	EndedAt      *time.Time `json:"endedAt,omitempty"`
}

// CreateSessionParams 是 POST /api/sessions 的入参（§2.3.6）。
// TaskID 可空（WS 直接启动会话时为 null）；Model 可空。
type CreateSessionParams struct {
	TaskID *string `json:"taskId,omitempty"`
	Cli    CliKind `json:"cli"`
	Model  string  `json:"model,omitempty"`
}
