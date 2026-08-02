package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/heycode/backend-go/internal/service"
)

// SnapshotHandler 处理 /api/sessions/:id/snapshots* 与 /api/snapshots/:id/rollback 路由（§2.3.7）。
type SnapshotHandler struct {
	svc *service.SnapshotService
}

func NewSnapshotHandler(svc *service.SnapshotService) *SnapshotHandler {
	return &SnapshotHandler{svc: svc}
}

// ListBySession GET /api/sessions/:sessionId/snapshots → {snapshots: FileSnapshot[]}
func (h *SnapshotHandler) ListBySession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	list, err := h.svc.ListBySession(r.Context(), sessionID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}

// ListByPath GET /api/sessions/:sessionId/snapshots/by-path?path= → {snapshots: FileSnapshot[]}
func (h *SnapshotHandler) ListByPath(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path 参数必填")
		return
	}
	list, err := h.svc.ListByPath(r.Context(), sessionID, path)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}

// RollbackSnapshot POST /api/snapshots/:snapshotId/rollback body:{serverId, cwd} → {result: RollbackResult}
func (h *SnapshotHandler) RollbackSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID := chi.URLParam(r, "snapshotId")
	var p service.RollbackParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateRollbackParams(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	result, err := h.svc.RollbackSnapshot(r.Context(), snapshotID, p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"result": result})
}

// RollbackSession POST /api/sessions/:sessionId/rollback body:{serverId, cwd} → {results: RollbackResult[]}
func (h *SnapshotHandler) RollbackSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	var p service.RollbackParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateRollbackParams(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	results, err := h.svc.RollbackSession(r.Context(), sessionID, p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"results": results})
}

func validateRollbackParams(p service.RollbackParams) map[string][]string {
	errs := make(map[string][]string)
	if p.ServerID == "" {
		errs["serverId"] = []string{"必填"}
	}
	if p.Cwd == "" {
		errs["cwd"] = []string{"必填"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
