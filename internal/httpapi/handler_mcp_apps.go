package httpapi

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	storepkg "chatdock/internal/store"

	"chatdock/internal/mcp"
)

type mcpAppToolCallRequest struct {
	SessionID  string         `json:"session_id"`
	SourceTool string         `json:"source_tool"`
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments"`
}

// handleCallMCPAppTool 只允许 App 调用产生它的同一个 MCP Server 中已暴露的工具。
// 目标调用仍经过 MCPClient 的 allow/deny 与 confirm 检查，App 不能借 host bridge 绕过授权。
func (a *Server) handleCallMCPAppTool(w http.ResponseWriter, r *http.Request) {
	var input mcpAppToolCallRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.SourceTool = strings.TrimSpace(input.SourceTool)
	input.Name = strings.TrimSpace(input.Name)
	if input.SessionID == "" || input.SourceTool == "" || input.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("session_id, source_tool and name are required"))
		return
	}
	if _, ok, err := a.store.GetSession(input.SessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	} else if !ok {
		writeError(w, http.StatusForbidden, fmt.Errorf("MCP App session is unavailable"))
		return
	}

	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	serverName, _, server, err := mcp.ResolveToolServer(cfg, input.SourceTool)
	if err != nil || server.Disabled {
		writeError(w, http.StatusForbidden, fmt.Errorf("MCP App source tool is unavailable"))
		return
	}

	// source_tool 来自父页面已完成的工具事件。这里再次核对其确实是该 Server
	// 当前允许暴露的工具，避免伪造 source_tool 只凭 server alias 获得桥接能力。
	tools, err := a.mcpClient.ListServerTools(r.Context(), cfg, serverName)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	sourceExposed := false
	for _, tool := range tools {
		if tool.FullName == input.SourceTool {
			sourceExposed = true
			break
		}
	}
	if !sourceExposed {
		writeError(w, http.StatusForbidden, fmt.Errorf("MCP App source tool is not exposed"))
		return
	}

	fullName := mcp.ToolFullName(serverName, input.Name)
	run, err := a.store.StartMCPRun(input.SessionID, "MCP App tool call")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	started := time.Now()
	if _, err := a.store.AddMCPRunEvent(run.ID, storepkg.RunEventInput{
		Kind: "tool_call", Status: "running", Tool: fullName, Arguments: input.Arguments, StartedAt: started,
	}); err != nil {
		_, _ = a.store.FinishMCPRun(run.ID, "failed", "MCP App tool call audit failed", err)
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	result, callErr := a.mcpClient.CallAppTool(r.Context(), cfg, serverName, input.Name, input.Arguments)
	finished := time.Now()
	status := "success"
	errorText := ""
	if callErr != nil {
		status = "failed"
		errorText = callErr.Error()
	}
	if _, auditErr := a.store.AddMCPRunEvent(run.ID, storepkg.RunEventInput{
		Kind: "tool_result", Status: status, Tool: fullName, Arguments: input.Arguments, Result: result, Error: errorText,
		StartedAt: started, FinishedAt: &finished, DurationMS: finished.Sub(started).Milliseconds(),
	}); auditErr != nil {
		log.Printf("MCP App tool result audit failed: session=%s tool=%s err=%v", input.SessionID, fullName, auditErr)
	}
	if _, finishErr := a.store.FinishMCPRun(run.ID, status, "MCP App tool call finished", callErr); finishErr != nil {
		log.Printf("MCP App run finish failed: session=%s tool=%s err=%v", input.SessionID, fullName, finishErr)
	}
	if callErr != nil {
		httpStatus := http.StatusBadGateway
		if errors.Is(callErr, mcp.ErrMCPAppToolForbidden) {
			httpStatus = http.StatusForbidden
		}
		writeError(w, httpStatus, callErr)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}
