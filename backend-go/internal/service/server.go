package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/heycode/backend-go/internal/ssh"
	"github.com/heycode/backend-go/internal/store"
	"github.com/heycode/backend-go/internal/types"
)

// ServerService 处理服务器 CRUD 与 SshAuth 加解密。
type ServerService struct {
	store     *store.ServerStore
	masterKey []byte
	// sshPool 由 main.go 注入；nil 时 Test 端点返回 503。
	// M2 阶段为 nil（CRUD 不依赖 SSH）。
	sshPool *ssh.Pool
}

// NewServerService 创建 ServerService（不含 SSH 池，适用于 M2 CRUD）。
func NewServerService(s *store.ServerStore, masterKey []byte) *ServerService {
	return &ServerService{store: s, masterKey: masterKey}
}

// WithSSHPool 注入 SSH 连接池，启用 Test 端点。
// 采用链式注入避免修改 NewServerService 签名（M2 测试不破坏）。
func (s *ServerService) WithSSHPool(pool *ssh.Pool) *ServerService {
	s.sshPool = pool
	return s
}

// CreateServerParams 是 POST /api/servers 的入参。
// Port 缺省 22；Auth 必填。
type CreateServerParams struct {
	Name     string          `json:"name"`
	Host     string          `json:"host"`
	Port     *int            `json:"port,omitempty"`
	Username string          `json:"username"`
	Auth     *types.SshAuth  `json:"auth"`
}

// UpdateServerParams 是 PATCH /api/servers/:id 的入参。所有字段可选。
type UpdateServerParams struct {
	Name     *string         `json:"name,omitempty"`
	Host     *string         `json:"host,omitempty"`
	Port     *int            `json:"port,omitempty"`
	Username *string         `json:"username,omitempty"`
	Auth     *types.SshAuth  `json:"auth,omitempty"`
}

// Create 创建服务器。
func (s *ServerService) Create(ctx context.Context, p CreateServerParams) (*types.ServerDTO, error) {
	if p.Auth == nil {
		return nil, errors.New("auth is required")
	}
	port := 22
	if p.Port != nil && *p.Port > 0 {
		port = *p.Port
	}
	encrypted, err := encryptAuth(s.masterKey, *p.Auth)
	if err != nil {
		return nil, err
	}
	sv := &store.Server{
		ID:            newID(),
		Name:          p.Name,
		Host:          p.Host,
		Port:          port,
		Username:      p.Username,
		AuthKind:      p.Auth.Kind,
		EncryptedAuth: encrypted,
		LastStatus:    types.ServerStatusUnknown,
		CreatedAt:     nowUTC(),
	}
	if err := s.store.Create(ctx, sv); err != nil {
		return nil, err
	}
	return serverToDTO(sv), nil
}

// Get 按 ID 查询。
func (s *ServerService) Get(ctx context.Context, id string) (*types.ServerDTO, error) {
	sv, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return serverToDTO(sv), nil
}

// List 返回全部服务器。
func (s *ServerService) List(ctx context.Context) ([]*types.ServerDTO, error) {
	list, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*types.ServerDTO, 0, len(list))
	for _, sv := range list {
		out = append(out, serverToDTO(sv))
	}
	return out, nil
}

// Update 部分更新。auth 为 nil 时保留原凭据；非 nil 时重新加密。
// 配置变更后失效 SSH 连接缓存，下次 acquire 用新配置重连。
func (s *ServerService) Update(ctx context.Context, id string, p UpdateServerParams) (*types.ServerDTO, error) {
	sv, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	if p.Name != nil {
		sv.Name = *p.Name
	}
	if p.Host != nil {
		sv.Host = *p.Host
	}
	if p.Port != nil && *p.Port > 0 {
		sv.Port = *p.Port
	}
	if p.Username != nil {
		sv.Username = *p.Username
	}
	if p.Auth != nil {
		encrypted, err := encryptAuth(s.masterKey, *p.Auth)
		if err != nil {
			return nil, err
		}
		sv.AuthKind = p.Auth.Kind
		sv.EncryptedAuth = encrypted
	}
	if err := s.store.Update(ctx, sv); err != nil {
		return nil, mapErr(err)
	}
	// 失效 SSH 连接（若已注入 pool）：host/port/username/auth 任一变更都会让旧连接不再合适
	if s.sshPool != nil {
		s.sshPool.Invalidate(id)
	}
	return serverToDTO(sv), nil
}

// Delete 删除服务器（级联删除 projects → tasks → sessions）。
// 同时失效 SSH 连接缓存，避免下次 acquire 拿到已不存在的 server。
func (s *ServerService) Delete(ctx context.Context, id string) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return mapErr(err)
	}
	if s.sshPool != nil {
		s.sshPool.Invalidate(id)
	}
	return nil
}

// GetDecryptedAuth 供 SSH 层（M3）使用：解密服务器凭据。
func (s *ServerService) GetDecryptedAuth(ctx context.Context, id string) (types.SshAuth, error) {
	sv, err := s.store.GetByID(ctx, id)
	if err != nil {
		return types.SshAuth{}, mapErr(err)
	}
	return decryptAuth(s.masterKey, sv.EncryptedAuth)
}

// MarkStatus 供 SSH 层连通性测试后更新状态。
func (s *ServerService) MarkStatus(ctx context.Context, id string, status types.ServerStatus, checkedAt time.Time) error {
	if err := s.store.UpdateStatus(ctx, id, status, &checkedAt); err != nil {
		return mapErr(err)
	}
	return nil
}

// ResolveServerAuth 实现 ssh.AuthResolver 接口。
// 返回建立 SSH 连接所需的全部信息（含解密后的凭据）。
// 资源不存在时返回 ssh.ErrServerNotFound，便于上层判断。
func (s *ServerService) ResolveServerAuth(ctx context.Context, serverID string) (ssh.ServerAuthInfo, error) {
	sv, err := s.store.GetByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ssh.ServerAuthInfo{}, ssh.ErrServerNotFound
		}
		return ssh.ServerAuthInfo{}, err
	}
	auth, err := decryptAuth(s.masterKey, sv.EncryptedAuth)
	if err != nil {
		return ssh.ServerAuthInfo{}, fmt.Errorf("decrypt auth: %w", err)
	}
	return ssh.ServerAuthInfo{
		Host:     sv.Host,
		Port:     sv.Port,
		Username: sv.Username,
		Auth:     auth,
	}, nil
}

// TestResult 是连通性测试结果（见 SPEC-GO-REWRITE.md §2.3.2）。
// Ok=true 时含 LatencyMs；Ok=false 时含 Error。
type TestResult struct {
	Ok        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Test 执行连通性测试：通过 sshPool 获取连接 → 执行 `echo __ok__` → 计 latencyMs。
// 成功/失败均更新 last_status / last_checked_at。
// sshPool 未注入时返回错误（handler 转 503）。
func (s *ServerService) Test(ctx context.Context, id string) (*TestResult, error) {
	if s.sshPool == nil {
		return nil, errors.New("ssh pool not configured")
	}
	start := time.Now()
	client, err := s.sshPool.Acquire(ctx, id)
	if err != nil {
		// 资源不存在单独返回
		if errors.Is(err, ssh.ErrServerNotFound) {
			return nil, ErrNotFound
		}
		// 连接失败：记录 fail，返回 {ok:false,error}
		_ = s.MarkStatus(ctx, id, types.ServerStatusFail, time.Now())
		return &TestResult{Ok: false, Error: err.Error()}, nil
	}

	// 已建立连接：执行 echo __ok__（5s 超时）
	res, err := ssh.Exec(ctx, client, "echo __ok__", ssh.ExecOptions{TimeoutMs: 5000})
	latency := time.Since(start).Milliseconds()
	if err != nil {
		_ = s.MarkStatus(ctx, id, types.ServerStatusFail, time.Now())
		return &TestResult{Ok: false, Error: err.Error()}, nil
	}
	// 校验输出含 __ok__（防止 hostkey banner 干扰）
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "__ok__") {
		_ = s.MarkStatus(ctx, id, types.ServerStatusFail, time.Now())
		return &TestResult{Ok: false, Error: fmt.Sprintf("unexpected output: exit=%d stdout=%q", res.ExitCode, strings.TrimSpace(res.Stdout))}, nil
	}

	_ = s.MarkStatus(ctx, id, types.ServerStatusOk, time.Now())
	return &TestResult{Ok: true, LatencyMs: latency}, nil
}

// serverToDTO 把 store 实体转换为对外 DTO。
// lastStatus 为 unknown 时不输出（DTO 字段为 nil）。
func serverToDTO(sv *store.Server) *types.ServerDTO {
	dto := &types.ServerDTO{
		ID:         sv.ID,
		Name:       sv.Name,
		Host:       sv.Host,
		Port:       sv.Port,
		Username:   sv.Username,
		AuthKind:   sv.AuthKind,
		CreatedAt:  sv.CreatedAt,
	}
	if sv.LastStatus != "" && sv.LastStatus != types.ServerStatusUnknown {
		st := sv.LastStatus
		dto.LastStatus = &st
	}
	dto.LastCheckedAt = sv.LastCheckedAt
	return dto
}
