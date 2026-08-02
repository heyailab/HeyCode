// Package ssh 封装 SSH 连接池、exec/spawnStream、SFTP 操作。
//
// 设计要点（见 SPEC-GO-REWRITE.md §2.7）：
//   - 连接池 map[serverId]*entry，entry 含 client、ready、configKey
//   - 保活：keepaliveInterval 30s，readyTimeout 15s
//   - 配置变更检测：比较 host/port/username/auth JSON，变更时关闭旧连接
//   - 重连：连接断开置 ready=false，下次 acquire 自动重连
//
// 依赖方向：ssh 包只依赖 types 与 AuthResolver 接口，
// 由 service 层实现 ResolveServerAuth，避免循环依赖。
package ssh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/heycode/backend-go/internal/types"
	_ssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// keepaliveInterval 是保活探测间隔。
const keepaliveInterval = 30 * time.Second

// readyTimeout 是 acquire 等待保活响应的超时。
const readyTimeout = 15 * time.Second

// AuthResolver 由 service 层实现，用于解析 serverId → 连接所需信息。
// 采用接口避免 ssh ↔ service 循环依赖。
type AuthResolver interface {
	// ResolveServerAuth 返回某服务器的连接信息与解密后的凭据。
	// 资源不存在时返回 ErrServerNotFound。
	ResolveServerAuth(ctx context.Context, serverID string) (ServerAuthInfo, error)
}

// ServerAuthInfo 是建立 SSH 连接所需的全部信息。
type ServerAuthInfo struct {
	Host     string
	Port     int
	Username string
	Auth     types.SshAuth
}

// ErrServerNotFound 由 AuthResolver 返回，表示服务器不存在。
var ErrServerNotFound = errors.New("server not found")

// Pool 是 SSH 连接池，per-serverId 复用 *ssh.Client。
//
// 线程安全：内部 mutex 保护 entries map；
// 每个 entry 自身 mutex 保护单 serverId 的并发 acquire/重连。
type Pool struct {
	resolver AuthResolver

	mu      sync.Mutex
	entries map[string]*entry
}

// entry 是单个 serverId 的连接状态。
type entry struct {
	mu        sync.Mutex // 串行化 acquire/重连
	client    *_ssh.Client
	ready     bool
	configKey string // host:port:username + auth JSON 的摘要，用于检测配置变更
}

// NewPool 创建连接池。
func NewPool(resolver AuthResolver) *Pool {
	return &Pool{
		resolver: resolver,
		entries:  make(map[string]*entry),
	}
}

// Acquire 获取某服务器的活跃 SSH 连接。
// 流程：
//  1. 解析 server auth
//  2. 取/建 entry，加锁
//  3. 配置变更检测：configKey 不一致则关闭旧连接
//  4. ready 检查：client 不为 nil 且未关闭 → 直接返回
//  5. 否则重连
func (p *Pool) Acquire(ctx context.Context, serverID string) (*_ssh.Client, error) {
	info, err := p.resolver.ResolveServerAuth(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrServerNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("resolve auth: %w", err)
	}
	key := configKey(info)

	e := p.getOrCreate(serverID)

	e.mu.Lock()
	defer e.mu.Unlock()

	// 配置变更检测
	if e.ready && e.configKey != key {
		_ = e.client.Close()
		e.client = nil
		e.ready = false
	}

	// 已就绪，校验连接活性后复用
	if e.ready && e.client != nil {
		if isAlive(e.client) {
			return e.client, nil
		}
		// 连接已断开，清理后重连
		_ = e.client.Close()
		e.client = nil
		e.ready = false
	}

	// 重连
	client, err := dial(info)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s@%s:%d: %w", info.Username, info.Host, info.Port, err)
	}
	e.client = client
	e.ready = true
	e.configKey = key

	// 启动保活 goroutine（per-connection）
	go keepalive(client)

	return client, nil
}

// Invalidate 失效某 serverId 的连接缓存。
// 在 server 更新/删除时调用，确保下次 acquire 用新配置重连。
func (p *Pool) Invalidate(serverID string) {
	p.mu.Lock()
	e, ok := p.entries[serverID]
	if ok {
		delete(p.entries, serverID)
	}
	p.mu.Unlock()

	if ok {
		e.mu.Lock()
		if e.client != nil {
			_ = e.client.Close()
		}
		e.ready = false
		e.mu.Unlock()
	}
}

// Close 关闭池中所有连接（优雅退出时调用）。
func (p *Pool) Close() {
	p.mu.Lock()
	entries := p.entries
	p.entries = make(map[string]*entry)
	p.mu.Unlock()

	for _, e := range entries {
		e.mu.Lock()
		if e.client != nil {
			_ = e.client.Close()
		}
		e.mu.Unlock()
	}
}

// getOrCreate 取或建 entry。Pool.mu 只保护 map 操作，不持有期间加 entry.mu。
func (p *Pool) getOrCreate(serverID string) *entry {
	p.mu.Lock()
	e, ok := p.entries[serverID]
	if !ok {
		e = &entry{}
		p.entries[serverID] = e
	}
	p.mu.Unlock()
	return e
}

// configKey 生成用于配置变更检测的摘要：host:port:username + auth JSON。
// auth JSON 包含 kind 与凭据字段，任一变更都会让 key 变化。
func configKey(info ServerAuthInfo) string {
	authJSON, _ := json.Marshal(info.Auth)
	return fmt.Sprintf("%s:%d:%s|%s", info.Host, info.Port, info.Username, string(authJSON))
}

// dial 建立 SSH 连接。
//   - password → ssh.Password
//   - privateKey → ssh.PublicKeys(signer) + passphrase
//   - agent → 通过 SSH_AUTH_SOCK 连接 agent（Unix；Windows 无 agent 时报错）
func dial(info ServerAuthInfo) (*_ssh.Client, error) {
	authMethod, err := authMethodFor(info.Auth)
	if err != nil {
		return nil, err
	}

	cfg := &_ssh.ClientConfig{
		User:            info.Username,
		Auth:            []_ssh.AuthMethod{authMethod},
		HostKeyCallback: _ssh.InsecureIgnoreHostKey(), // HeyCode 自托管场景：用户自管服务器
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(info.Host, fmt.Sprintf("%d", info.Port))
	client, err := _ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// authMethodFor 根据认证类型构造 ssh.AuthMethod。
func authMethodFor(auth types.SshAuth) (_ssh.AuthMethod, error) {
	switch auth.Kind {
	case types.AuthPassword:
		return _ssh.Password(auth.Password), nil

	case types.AuthPrivateKey:
		var signer _ssh.Signer
		var err error
		if auth.Passphrase != "" {
			signer, err = _ssh.ParsePrivateKeyWithPassphrase([]byte(auth.PrivateKey), []byte(auth.Passphrase))
		} else {
			signer, err = _ssh.ParsePrivateKey([]byte(auth.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return _ssh.PublicKeys(signer), nil

	case types.AuthAgent:
		// 通过 SSH_AUTH_SOCK 连接本机 agent。Windows 无 Unix socket 支持，会报错。
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return nil, errors.New("agent auth requires SSH_AUTH_SOCK env var")
		}
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return nil, fmt.Errorf("dial agent socket: %w", err)
		}
		ag := agent.NewClient(conn)
		signers, err := ag.Signers()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("agent signers: %w", err)
		}
		return _ssh.PublicKeys(signers...), nil

	default:
		return nil, fmt.Errorf("unsupported auth kind: %s", auth.Kind)
	}
}

// isAlive 用 SendRequest 同步探测连接是否还活着。
func isAlive(client *_ssh.Client) bool {
	_, _, err := client.SendRequest("keepalive@golang.org", true, nil)
	return err == nil
}

// keepalive 周期性发送 keepalive 请求；连接断开时退出。
// 不主动关闭 client（由 acquire 检测到 not alive 后清理）。
func keepalive(client *_ssh.Client) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for range ticker.C {
		_, _, err := client.SendRequest("keepalive@golang.org", true, nil)
		if err != nil {
			return
		}
	}
}
