package chatdock

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	storepkg "chatdock/internal/chatdock/store"
)

func (a *App) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	setup, err := a.store.SetupStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	data, err := a.store.DataStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{
		"ok":       true,
		"name":     "ChatDock",
		"time":     time.Now(),
		"setup":    setup,
		"data":     data,
		"web_dir":  a.cfg.WebDir,
		"addr":     a.cfg.Addr,
		"database": filepath.Base(data.DatabasePath),
	})
}

func (a *App) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.activeMCPConfig(a.workspaceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]storepkg.MCPServerStatus, 0, len(names))
	for _, name := range names {
		server := cfg.Servers[name]
		status := storepkg.MCPServerStatus{
			Name:         name,
			URL:          server.URL,
			Disabled:     server.Disabled,
			AuthType:     server.Auth.Type,
			HasToken:     server.BearerToken() != "",
			AllowCount:   len(server.AllowTools),
			DenyCount:    len(server.DenyTools),
			ConfirmCount: len(server.ConfirmTools),
			TimeoutMS:    server.TimeoutMS,
			CacheTTLMS:   server.CacheTTLMS,
			LastStatus:   "unknown",
		}
		if server.Disabled || strings.TrimSpace(server.URL) == "" {
			status.LastStatus = "disabled"
		} else if tools, err := a.mcpClient.ListServerTools(r.Context(), cfg, name); err != nil {
			status.LastStatus = "error"
			status.LastError = err.Error()
		} else {
			status.LastStatus = fmt.Sprintf("ok · %d tools", len(tools))
		}
		servers = append(servers, status)
	}
	writeJSONResponse(w, http.StatusOK, storepkg.MCPStatusResponse{Servers: servers})
}
