package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/heycode/backend-go/internal/service"
)

// FileHandler 处理 /api/servers/{id}/files* 路由。
type FileHandler struct {
	svc *service.FileService
}

func NewFileHandler(svc *service.FileService) *FileHandler {
	return &FileHandler{svc: svc}
}

// List GET /api/servers/{id}/files?path=<dir>
// query.path 必填；缺失返回 400。
func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	dir := r.URL.Query().Get("path")
	if dir == "" {
		respondError(w, http.StatusBadRequest, "path 参数必填")
		return
	}
	listing, err := h.svc.List(r.Context(), serverID, dir)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, listing)
}

// Read GET /api/servers/{id}/files/content?path=<file>
func (h *FileHandler) Read(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		respondError(w, http.StatusBadRequest, "path 参数必填")
		return
	}
	content, err := h.svc.Read(r.Context(), serverID, filePath)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, content)
}

// WriteBody 是 PUT /api/servers/{id}/files/content 的请求体。
type WriteBody struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Write PUT /api/servers/{id}/files/content
// body: {path, content}；path 必填。
func (h *FileHandler) Write(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	var body WriteBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Path == "" {
		respondError(w, http.StatusBadRequest, "path 必填")
		return
	}
	result, err := h.svc.Write(r.Context(), serverID, body.Path, body.Content)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// DeleteBody 是 DELETE /api/servers/{id}/files 的请求体。
type DeleteBody struct {
	Path string `json:"path"`
}

// Delete DELETE /api/servers/{id}/files
// body: {path}；path 必填。
func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	var body DeleteBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Path == "" {
		respondError(w, http.StatusBadRequest, "path 必填")
		return
	}
	if err := h.svc.Delete(r.Context(), serverID, body.Path); err != nil {
		respondServiceError(w, err)
		return
	}
	respondOK(w)
}
