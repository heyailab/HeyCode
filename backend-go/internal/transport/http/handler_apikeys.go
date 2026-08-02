package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/heycode/backend-go/internal/service"
	"github.com/heycode/backend-go/internal/types"
)

// ApiKeyHandler 处理 /api/api-keys* 路由。
type ApiKeyHandler struct {
	svc *service.ApiKeyService
}

func NewApiKeyHandler(svc *service.ApiKeyService) *ApiKeyHandler {
	return &ApiKeyHandler{svc: svc}
}

// List GET /api/api-keys
// 返回 6 个支持 cli 的 ApiKeyMeta，未配置的 hasKey=false。
func (h *ApiKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

// Upsert POST /api/api-keys
// 请求体 {cli, key}；不支持的 cli 返回 400。
func (h *ApiKeyHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var p service.UpsertApiKeyParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateUpsertApiKey(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	meta, err := h.svc.Upsert(r.Context(), p)
	if err != nil {
		// 不支持的 cli 或 key 为空 → 400；其它 → 500
		if err.Error() == "unsupported cli: "+p.Cli || err.Error() == "key is required" {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, meta)
}

// Delete DELETE /api/api-keys/{cli}
// 不支持的 cli 返回 400；未配置返回 404。
func (h *ApiKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cliWire := chi.URLParam(r, "cli")
	cli := types.CliKind(cliWire)
	if !types.IsSupportedCliKind(cli) {
		respondError(w, http.StatusBadRequest, "不支持的 cli: "+cliWire)
		return
	}
	if err := h.svc.Delete(r.Context(), cli); err != nil {
		respondServiceError(w, err)
		return
	}
	respondOK(w)
}

func validateUpsertApiKey(p service.UpsertApiKeyParams) map[string][]string {
	errs := make(map[string][]string)
	if p.Cli == "" {
		errs["cli"] = []string{"必填"}
	}
	if p.Key == "" {
		errs["key"] = []string{"必填"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
