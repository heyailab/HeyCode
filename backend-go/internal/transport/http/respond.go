package http

import (
	"encoding/json"
	"net/http"

	"github.com/heycode/backend-go/internal/service"
)

// okBody 是 {ok:true} 风格响应。
type okBody struct {
	OK bool `json:"ok"`
}

// errorBody 是 {"error":"..."} 风格响应。
// Error 字段为 any 以支持两种形态：
//   - 简单错误：string
//   - 校验错误：{formErrors:[], fieldErrors:{...}}
type errorBody struct {
	Error any `json:"error"`
}

// respondJSON 写入指定状态码 + JSON body。
func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// respondOK 写入 200 + {ok:true}。
func respondOK(w http.ResponseWriter) {
	respondJSON(w, http.StatusOK, okBody{OK: true})
}

// respondError 写入简单字符串错误。
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, errorBody{Error: msg})
}

// respondFieldErrors 写入校验错误结构。
func respondFieldErrors(w http.ResponseWriter, status int, fieldErrors map[string][]string) {
	respondJSON(w, status, errorBody{Error: map[string]any{
		"formErrors":  []string{},
		"fieldErrors": fieldErrors,
	}})
}

// respondServiceError 把 service 层错误映射为合适的 HTTP 响应。
// ErrNotFound → 404；其它 → 500。
func respondServiceError(w http.ResponseWriter, err error) {
	if err == service.ErrNotFound {
		respondError(w, http.StatusNotFound, "资源不存在")
		return
	}
	respondError(w, http.StatusInternalServerError, err.Error())
}

// decodeJSON 解析请求体到 v。解析失败返回 false（已写错误响应）。
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		respondError(w, http.StatusBadRequest, "请求体为空")
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		respondError(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return false
	}
	return true
}
