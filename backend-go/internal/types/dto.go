package types

import "time"

// SshAuth 是判别联合，通过 Kind 字段区分具体认证方式。
// 序列化为 JSON 时按 kind 输出对应字段（omitempty 隐藏空字段）。
//
//	{kind:"password",password}
//	{kind:"privateKey",privateKey,passphrase?}
//	{kind:"agent"}
type SshAuth struct {
	Kind       SshAuthKind `json:"kind"`
	Password   string      `json:"password,omitempty"`
	PrivateKey string      `json:"privateKey,omitempty"`
	Passphrase string      `json:"passphrase,omitempty"`
}

// ServerDTO 是 Server 的对外响应 DTO。
// 绝不返回明文凭据：只有 authKind，没有 password/privateKey 等。
//
// lastStatus 仅在非 unknown 时输出；lastCheckedAt 仅在非 nil 时输出。
type ServerDTO struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Host          string        `json:"host"`
	Port          int           `json:"port"`
	Username      string        `json:"username"`
	AuthKind      SshAuthKind   `json:"authKind"`
	CreatedAt     time.Time     `json:"createdAt"`
	LastStatus    *ServerStatus `json:"lastStatus,omitempty"`
	LastCheckedAt *time.Time    `json:"lastCheckedAt,omitempty"`
}

// ProjectDTO 是 Project 的对外响应 DTO。
type ProjectDTO struct {
	ID           string    `json:"id"`
	ServerID     string    `json:"serverId"`
	Name         string    `json:"name"`
	Cwd          string    `json:"cwd"`
	DefaultCli   CliKind   `json:"defaultCli"`
	DefaultModel string    `json:"defaultModel,omitempty"`
	Rules        string    `json:"rules,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// TaskDTO 是 Task 的对外响应 DTO。
type TaskDTO struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// ApiKeyMeta 是 API Key 的对外响应 DTO（绝不返回密钥明文）。
type ApiKeyMeta struct {
	Cli       CliKind    `json:"cli"`
	HasKey    bool       `json:"hasKey"`
	Last4     string     `json:"last4,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}
