package http

import (
	"encoding/json"
	"net/http"

	"github.com/heycode/backend-go/internal/auth"
)

// AuthHandler 处理 /api/auth/* 路由。
type AuthHandler struct {
	mgr *auth.Manager
}

// NewAuthHandler 创建 AuthHandler。mgr 可为 nil（鉴权未启用）。
func NewAuthHandler(mgr *auth.Manager) *AuthHandler {
	return &AuthHandler{mgr: mgr}
}

// Verify POST /api/auth/verify
//
// 客户端携带 Authorization: Bearer <token> 调用本端点验证鉴权是否通过。
// 该端点本身被鉴权中间件保护（若启用），因此：
//   - 鉴权未启用 → 200 {ok:true, authEnabled:false}
//   - 鉴权启用且 token 正确 → 200 {ok:true, authEnabled:true}
//   - 鉴权启用但 token 缺失/错误 → 由中间件返回 401（不会进入此 handler）
//
// App 设置页"测试连接"应调用此端点（而非 /api/health），
// 以同时验证 URL 可达性与 token 正确性。
func (h *AuthHandler) Verify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enabled := h.mgr != nil && h.mgr.Enabled()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":          true,
		"authEnabled": enabled,
		"version":     Version,
	})
}
