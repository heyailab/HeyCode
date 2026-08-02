package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/heycode/backend-go/internal/service"
)

// ProjectHandler 处理 /api/projects* 路由。
type ProjectHandler struct {
	svc *service.ProjectService
}

func NewProjectHandler(svc *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

// List GET /api/projects?serverId=
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID := r.URL.Query().Get("serverId")
	list, err := h.svc.List(r.Context(), serverID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

// Create POST /api/projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p service.CreateProjectParams
	if !decodeJSON(w, r, &p) {
		return
	}
	if verr := validateCreateProject(p); verr != nil {
		respondFieldErrors(w, http.StatusBadRequest, verr)
		return
	}
	proj, err := h.svc.Create(r.Context(), p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, proj)
}

// Get GET /api/projects/{id}
func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	proj, err := h.svc.Get(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, proj)
}

// Update PATCH /api/projects/{id}
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var p service.UpdateProjectParams
	if !decodeJSON(w, r, &p) {
		return
	}
	proj, err := h.svc.Update(r.Context(), id, p)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, proj)
}

// Delete DELETE /api/projects/{id}
func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		respondServiceError(w, err)
		return
	}
	respondOK(w)
}

func validateCreateProject(p service.CreateProjectParams) map[string][]string {
	errs := make(map[string][]string)
	if p.ServerID == "" {
		errs["serverId"] = []string{"必填"}
	}
	if p.Name == "" {
		errs["name"] = []string{"必填"}
	}
	if p.Cwd == "" {
		errs["cwd"] = []string{"必填"}
	}
	if p.DefaultCli == "" {
		errs["defaultCli"] = []string{"必填"}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
