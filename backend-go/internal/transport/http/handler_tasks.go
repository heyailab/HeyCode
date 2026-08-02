package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/heycode/backend-go/internal/service"
)

// TaskHandler 处理 /api/tasks* 与 /api/projects/{id}/tasks 路由。
type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

// ListByProject GET /api/projects/{id}/tasks
func (h *TaskHandler) ListByProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	list, err := h.svc.ListByProject(r.Context(), projectID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

// Create POST /api/tasks
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p service.CreateTaskParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateCreateTask(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	t, err := h.svc.Create(r.Context(), p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, t)
}

// Get GET /api/tasks/{id}
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, t)
}

// Update PATCH /api/tasks/{id}
func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p service.UpdateTaskParams
	if !decodeJSON(w, r, &p) {
		return
	}
	t, err := h.svc.Update(r.Context(), id, p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, t)
}

// Delete DELETE /api/tasks/{id}
func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		respondServiceError(w, err)
		return
	}
	respondOK(w)
}

func validateCreateTask(p service.CreateTaskParams) map[string][]string {
	errs := make(map[string][]string)
	if p.ProjectID == "" {
		errs["projectId"] = []string{"必填"}
	}
	if p.Title == "" {
		errs["title"] = []string{"必填"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
