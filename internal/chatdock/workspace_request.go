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
		// 前端会把当前工作空间显式放到每个请求头里；旧 Store 仍有少量当前工作空间缓存，
		// 因此每个请求都必须同步缓存，包括 default。不能吞掉无效 workspace 错误，否则会继续沿用
		// 上一次请求留下的缓存，造成跨工作空间串读写。
		if _, err := a.store.LoadWorkspaceCache(model.WorkspaceIDRequest{Name: workspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
