// Package auth 提供 Bearer token 鉴权中间件。
//
// 设计：预共享密钥（pre-shared key）模型，适合自托管单用户场景。
//   - 后端启动时从 AUTH_TOKEN 环境变量加载（逗号分隔支持多 token，便于轮换）
//   - 客户端每个请求带 Authorization: Bearer <token>
//   - WebSocket 走 ?token=<token> query param（浏览器 WS 不能设 header）
//   - AUTH_TOKEN 未配置时 Enable() 返回 false，中间件不启用（兼容本地开发）
//
// 安全：
//   - 用 crypto/subtle.ConstantTimeCompare 防时序攻击
//   - 校验失败统一返回 401，不区分"缺 header"与"token 错"避免信息泄露
package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Manager 持有已配置的 token 列表。
// tokens 为空表示鉴权未启用（所有请求放行）。
type Manager struct {
	tokens [][]byte
}

// New 创建 Manager。tokens 为空切片时鉴权关闭。
// 入参会被规范化：空白项被丢弃，前后空白被去除。
func New(tokens []string) *Manager {
	cleaned := make([][]byte, 0, len(tokens))
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t != "" {
			cleaned = append(cleaned, []byte(t))
		}
	}
	return &Manager{tokens: cleaned}
}

// Enabled 返回鉴权是否启用（至少配置了一个 token）。
func (m *Manager) Enabled() bool {
	return len(m.tokens) > 0
}

// verify 用常量时间比较校验 token。
func (m *Manager) verify(token string) bool {
	if !m.Enabled() {
		return true
	}
	t := []byte(token)
	for _, valid := range m.tokens {
		// 长度不同也走一次比较，避免基于长度的时序侧信道
		if subtle.ConstantTimeCompare(t, valid) == 1 {
			return true
		}
	}
	return false
}

// extractBearer 从 Authorization 头提取 Bearer token。
// 格式：Authorization: Bearer <token>
// 返回空字符串表示缺失或格式错误。
func extractBearer(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// Middleware 返回 chi 兼容的鉴权中间件。
// 鉴权未启用时返回 nil（调用方据此决定是否 Use）。
func (m *Manager) Middleware() func(http.Handler) http.Handler {
	if !m.Enabled() {
		return nil
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r.Header.Get("Authorization"))
			if token == "" || !m.verify(token) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// VerifyQuery 从 query string 的 token 参数校验，供 WebSocket 使用。
// 鉴权未启用时直接返回 true。
func (m *Manager) VerifyQuery(r *http.Request) bool {
	if !m.Enabled() {
		return true
	}
	return m.verify(r.URL.Query().Get("token"))
}
