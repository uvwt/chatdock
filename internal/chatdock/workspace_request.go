package chatdock

import (
	"fmt"
	"net/http"
	"strings"
)

func (a *App) workspaceIDFromRequest(r *http.Request) string {
	for _, value := range []string{r.URL.Query().Get("workspace_id"), r.Header.Get("X-Workspace-ID"), r.Header.Get("X-ChatDock-Workspace-ID")} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "default"
}

func (a *App) workspaceIDForSession(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "default", nil
	}
	workspaceID, ok, err := a.store.SessionWorkspace(sessionID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	return workspaceID, nil
}

func (a *App) workspaceScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 工作空间集合是恢复入口：客户端 localStorage 可能仍保存已删除的空间 ID，
		// GET/POST 必须先能返回真实列表或创建新空间，其他业务路由仍严格校验作用域。
		if r.URL.Path == "/api/workspaces" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
			next.ServeHTTP(w, r)
			return
		}
		workspaceID := a.workspaceIDFromRequest(r)
		if err := a.store.RequireWorkspace(workspaceID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
