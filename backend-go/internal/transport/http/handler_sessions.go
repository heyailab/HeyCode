package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/heycode/backend-go/internal/service"
	"github.com/heycode/backend-go/internal/types"
)

// SessionHandler 处理 /api/sessions* 与 /api/tasks/:id/sessions 路由（§2.3.6）。
type SessionHandler struct {
	svc *service.SessionService
}

func NewSessionHandler(svc *service.SessionService) *SessionHandler {
	return &SessionHandler{svc: svc}
}

// ListByTask GET /api/tasks/:id/sessions
func (h *SessionHandler) ListByTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	list, err := h.svc.ListByTask(r.Context(), taskID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

// Create POST /api/sessions → Session（status=idle，§4.14）
func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p types.CreateSessionParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateCreateSession(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	sess, err := h.svc.Create(r.Context(), p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, sess)
}

// Get GET /api/sessions/:id → Session / 404
func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := h.svc.Get(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, sess)
}

// Events GET /api/sessions/:id/events?since=N → {events: ServerEnvelope[]}
func (h *SessionHandler) Events(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)

	events, err := h.svc.GetEvents(r.Context(), id, since)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	// spec §2.3.6：响应是 {events: ServerEnvelope[]}
	respondJSON(w, http.StatusOK, map[string]any{"events": events})
}

// Delete DELETE /api/sessions/:id → {ok}
func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		respondServiceError(w, err)
		return
	}
	respondOK(w)
}

func validateCreateSession(p types.CreateSessionParams) map[string][]string {
	errs := make(map[string][]string)
	if p.Cli == "" {
		errs["cli"] = []string{"必填"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
