package http

import (
	"encoding/json"
	"net/http"
)

// Health 处理 GET /api/health。
//
// 响应：200 OK
//
//	{"ok": true, "version": "0.2.0"}
//
// App 设置页"测试连接"会读 version 字段显示。
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"version": Version,
	})
}
