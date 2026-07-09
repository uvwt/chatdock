package chatdock

import (
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

func (a *App) workspaceIDForSession(sessionID string) string {
	if workspaceID, ok, err := a.store.SessionWorkspace(strings.TrimSpace(sessionID)); err == nil && ok {
		return workspaceID
	}
	return "default"
}

func (a *App) workspaceScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaceID := a.workspaceIDFromRequest(r)
		if err := a.store.RequireWorkspace(workspaceID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
