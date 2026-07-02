package chatdock

import (
	"fmt"
	"net/http"
	"strings"
)

func (a *App) handleGetMCPConfig(w http.ResponseWriter, r *http.Request) {
	content, err := a.store.GetMCPConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPConfigResponse{Content: content})
}

func (a *App) handleSaveMCPConfig(w http.ResponseWriter, r *http.Request) {
	var input SaveMCPConfigRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	content, err := a.store.SaveMCPConfig(input.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPConfigResponse{Content: content})
}

func (a *App) activeMCPConfig() (MCPConfig, error) {
	content, err := a.store.GetEffectiveMCPConfig()
	if err != nil {
		return MCPConfig{}, err
	}
	return ParseMCPConfig(content)
}

func (a *App) handleListMCPTools(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tools, err := a.mcpClient.ListTools(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPToolsResponse{Tools: tools})
}

func (a *App) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	serverName := strings.TrimSpace(r.URL.Query().Get("server"))
	if serverName == "" {
		serverName = "agentdock"
	}
	tools, err := a.mcpClient.ListServerTools(r.Context(), cfg, serverName)
	if err != nil {
		writeJSONResponse(w, http.StatusBadGateway, map[string]any{"ok": false, "server": serverName, "error": err.Error()})
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "server": serverName, "tool_count": len(tools), "tools": tools})
}

func (a *App) handleCallMCPTool(w http.ResponseWriter, r *http.Request) {
	var input MCPToolCallRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("tool name is empty"))
		return
	}
	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.mcpClient.CallTool(r.Context(), cfg, input.Name, input.Arguments)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPToolCallResponse{Name: input.Name, Result: result})
}
