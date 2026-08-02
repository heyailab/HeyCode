package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/heycode/backend-go/internal/service"
	"github.com/heycode/backend-go/internal/types"
)

// ServerHandler 处理 /api/servers* 路由。
type ServerHandler struct {
	svc *service.ServerService
}

func NewServerHandler(svc *service.ServerService) *ServerHandler {
	return &ServerHandler{svc: svc}
}

// List GET /api/servers?projectId=（projectId 参数当前忽略，保留兼容）
func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

// Create POST /api/servers
func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p service.CreateServerParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateCreateServer(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	sv, err := h.svc.Create(r.Context(), p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, sv)
}

// Get GET /api/servers/{id}
func (h *ServerHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sv, err := h.svc.Get(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, sv)
}

// Update PATCH /api/servers/{id}
func (h *ServerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p service.UpdateServerParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateUpdateServer(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	sv, err := h.svc.Update(r.Context(), id, p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, sv)
}

// Delete DELETE /api/servers/{id}
func (h *ServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		respondServiceError(w, err)
		return
	}
	respondOK(w)
}

// Test POST /api/servers/{id}/test
// 执行 SSH 连通性测试：echo __ok__，返回 {ok:true,latencyMs} 或 {ok:false,error}。
// sshPool 未注入时返回 503（M2 兼容场景）。
func (h *ServerHandler) Test(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.svc.Test(r.Context(), id)
	if err != nil {
		// sshPool 未配置 → 503；资源不存在 → 404；其它 → 500
		if err == service.ErrNotFound {
			respondServiceError(w, err)
			return
		}
		respondError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	// ok=true → 200；ok=false → 200（业务层失败而非协议错误，App 仍读 body 判断）
	// spec §2.3.2 仅约定响应体形态，未约定状态码区分 ok true/false
	respondJSON(w, http.StatusOK, result)
}

// validateCreateServer 校验创建参数，返回 fieldErrors（nil 表示通过）。
func validateCreateServer(p service.CreateServerParams) map[string][]string {
	errs := make(map[string][]string)
	if p.Name == "" {
		errs["name"] = []string{"必填"}
	}
	if p.Host == "" {
		errs["host"] = []string{"必填"}
	}
	if p.Username == "" {
		errs["username"] = []string{"必填"}
	}
	if p.Auth == nil {
		errs["auth"] = []string{"必填"}
	} else if !isValidAuthKind(p.Auth.Kind) {
		errs["auth"] = []string{"kind 必须是 password/privateKey/agent"}
	} else if p.Auth.Kind == types.AuthPassword && p.Auth.Password == "" {
		errs["auth"] = []string{"password 必填"}
	} else if p.Auth.Kind == types.AuthPrivateKey && p.Auth.PrivateKey == "" {
		errs["auth"] = []string{"privateKey 必填"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// validateUpdateServer 校验更新参数。auth 提供时校验 kind 合法性。
func validateUpdateServer(p service.UpdateServerParams) map[string][]string {
	if p.Auth == nil {
		return nil
	}
	if !isValidAuthKind(p.Auth.Kind) {
		return map[string][]string{"auth": {"kind 必须是 password/privateKey/agent"}}
	}
	if p.Auth.Kind == types.AuthPassword && p.Auth.Password == "" {
		return map[string][]string{"auth": {"password 必填"}}
	}
	if p.Auth.Kind == types.AuthPrivateKey && p.Auth.PrivateKey == "" {
		return map[string][]string{"auth": {"privateKey 必填"}}
	}
	return nil
}

func isValidAuthKind(k types.SshAuthKind) bool {
	switch k {
	case types.AuthPassword, types.AuthPrivateKey, types.AuthAgent:
		return true
	}
	return false
}
