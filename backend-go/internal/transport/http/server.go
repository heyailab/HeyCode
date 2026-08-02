// Package http 是后端 HTTP 传输层，基于 chi 路由器装配所有 REST 端点。
//
// M1：/api/health
// M2：/api/servers、/api/projects、/api/tasks、/api/api-keys
// 后续：/api/servers/{id}/files、/api/sessions、/api/sessions/{id}/snapshots、/ws/sessions/{id}
// M10：/api/auth/verify + Bearer token 鉴权中间件
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/heycode/backend-go/internal/auth"
)

// Version 是后端 API 版本号，与 /api/health 响应中的 version 字段对齐。
const Version = "0.3.0"

// Dependencies 装配路由所需的各领域 handler。
type Dependencies struct {
	Servers   *ServerHandler
	Projects  *ProjectHandler
	Tasks     *TaskHandler
	ApiKeys   *ApiKeyHandler
	Files     *FileHandler     // M3
	Sessions  *SessionHandler  // M6
	Snapshots *SnapshotHandler // M7
	Auth      *AuthHandler     // M10
	WS        http.Handler     // M6：/ws/sessions/:sessionId
}

// NewRouter 装配 chi 路由器。
//
// 鉴权策略（M10）：
//   - AuthMgr 为 nil 或未启用 → 不挂中间件（兼容本地开发）
//   - 启用时，/api/health 和 /api/auth/verify 保持公开，其余 /api/* 需 Bearer token
//   - WebSocket 由 deps.WS 内部自行调用 AuthMgr.VerifyQuery 校验 ?token=
func NewRouter(deps Dependencies, authMgr *auth.Manager) http.Handler {
	r := chi.NewRouter()

	// 中间件
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// 不套全局超时：chi middleware.Timeout(0) 会立即让 ctx 超时（v5.1.0 无 d==0 短路），
	// 且 WS 等长连接不能套全局超时。由各 handler 自行控制。

	// 鉴权中间件（启用时才挂）
	authMW := authMgr.Middleware()

	r.Route("/api", func(r chi.Router) {
		// 公开端点：健康检查、鉴权自检
		r.Get("/health", Health)
		if deps.Auth != nil {
			// /api/auth/verify 路由本身不需要 token；
			// 客户端带 token 调用时，由中间件放行后进入 handler 返回 authEnabled=true。
			// 响应只暴露 authEnabled 与 version，不泄露敏感信息。
			r.Route("/auth", func(r chi.Router) {
				r.Post("/verify", deps.Auth.Verify)
				r.Get("/verify", deps.Auth.Verify) // 兼容 GET 便于浏览器直接测试
			})
		}

		// 受保护路由组：servers/projects/tasks/sessions/snapshots/api-keys
		// 用 r.Group 局部挂中间件，不影响上面的公开端点。
		r.Group(func(r chi.Router) {
			if authMW != nil {
				r.Use(authMW)
			}
			registerProtectedRoutes(r, deps)
		})
	})

	// WebSocket (M6)：/ws/sessions/:sessionId
	// 鉴权由 WS handler 内部用 AuthMgr.VerifyQuery 校验 ?token=，
	// 因 chi 中间件无法在 Upgrade 前拦截 query param。
	if deps.WS != nil {
		r.Get("/ws/sessions/{sessionId}", deps.WS.ServeHTTP)
	}

	return r
}

// registerProtectedRoutes 在已挂鉴权中间件的子路由器上注册业务端点。
func registerProtectedRoutes(r chi.Router, deps Dependencies) {
	// Servers
	r.Route("/servers", func(r chi.Router) {
		r.Get("/", deps.Servers.List)
		r.Post("/", deps.Servers.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", deps.Servers.Get)
			r.Patch("/", deps.Servers.Update)
			r.Delete("/", deps.Servers.Delete)
			r.Post("/test", deps.Servers.Test)

			// Files (SFTP) — M3
			if deps.Files != nil {
				r.Get("/files", deps.Files.List)
				r.Get("/files/content", deps.Files.Read)
				r.Put("/files/content", deps.Files.Write)
				r.Delete("/files", deps.Files.Delete)
			}
		})
	})

	// Projects
	r.Route("/projects", func(r chi.Router) {
		r.Get("/", deps.Projects.List)
		r.Post("/", deps.Projects.Create)
		// /api/projects/{id}/tasks（任务列表按项目过滤）
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", deps.Projects.Get)
			r.Patch("/", deps.Projects.Update)
			r.Delete("/", deps.Projects.Delete)
			r.Get("/tasks", deps.Tasks.ListByProject)
		})
	})

	// Tasks
	r.Route("/tasks", func(r chi.Router) {
		r.Post("/", deps.Tasks.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", deps.Tasks.Get)
			r.Patch("/", deps.Tasks.Update)
			r.Delete("/", deps.Tasks.Delete)
			// M6：任务下的会话列表
			if deps.Sessions != nil {
				r.Get("/sessions", deps.Sessions.ListByTask)
			}
		})
	})

	// Sessions (M6) + Snapshots (M7)
	if deps.Sessions != nil {
		r.Route("/sessions", func(r chi.Router) {
			r.Post("/", deps.Sessions.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", deps.Sessions.Get)
				r.Get("/events", deps.Sessions.Events)
				r.Delete("/", deps.Sessions.Delete)
				// M7：会话级快照与回滚
				if deps.Snapshots != nil {
					r.Get("/snapshots", deps.Snapshots.ListBySession)
					r.Get("/snapshots/by-path", deps.Snapshots.ListByPath)
					r.Post("/rollback", deps.Snapshots.RollbackSession)
				}
			})
		})
	}

	// Snapshots 单条回滚 (M7)
	if deps.Snapshots != nil {
		r.Route("/snapshots", func(r chi.Router) {
			r.Post("/{snapshotId}/rollback", deps.Snapshots.RollbackSnapshot)
		})
	}

	// API Keys
	r.Route("/api-keys", func(r chi.Router) {
		r.Get("/", deps.ApiKeys.List)
		r.Post("/", deps.ApiKeys.Upsert)
		r.Delete("/{cli}", deps.ApiKeys.Delete)
	})
}
