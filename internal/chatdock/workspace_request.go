package chatdock

import (
	"net/http"
	"strings"

	"chatdock/internal/chatdock/model"
)

func (a *App) workspaceIDFromRequest(r *http.Request) string {
	for _, value := range []string{r.URL.Query().Get("workspace_id"), r.Header.Get("X-Workspace-ID"), r.Header.Get("X-ChatDock-Workspace-ID")} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "default"
}

func (a *App) workspaceIDForSession(sessionID string) string {
	if workspaceID, ok, err := a.store.SessionWorkspace(strings.TrimSpace(sessionID)); err == nil && ok {
		return workspaceID
	}
	return "default"
}

func (a *App) workspaceScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaceID := a.workspaceIDFromRequest(r)
		if workspaceID != "" && workspaceID != "default" {
			// 前端把当前工作空间显式放到每个请求头里；旧 Store 仍有少量当前工作空间缓存，
			// 因此进入 handler 前先把缓存同步到请求指定的 workspace。无效 workspace 不在这里报错，
			// 交给具体 handler 返回更贴近业务的错误。
			_, _ = a.store.LoadWorkspaceCache(model.WorkspaceIDRequest{Name: workspaceID})
		}
		next.ServeHTTP(w, r)
	})
}
