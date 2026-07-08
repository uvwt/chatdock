package chatdock

import (
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type MCPConfirmation struct {
	ID          string         `json:"id"`
	Workspace   string         `json:"workspace"`
	SessionID   string         `json:"session_id,omitempty"`
	Tool        string         `json:"tool"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Status      string         `json:"status"`
	RequestedAt time.Time      `json:"requested_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
	Message     string         `json:"message,omitempty"`
	decision    chan bool
}

type MCPConfirmationResolveRequest struct {
	Approve bool `json:"approve"`
}

func mcpToolNeedsConfirmation(cfg mcp.MCPConfig, fullName string) bool {
	serverName, toolName := mcp.SplitToolFullName(fullName)
	server, ok := cfg.Servers[serverName]
	return ok && server.RequiresConfirmation(toolName, fullName)
}

func (a *App) requestMCPConfirmation(ctx context.Context, sessionID string, tool string, args map[string]any, emit func(string, any) error) error {
	workspaceID := a.workspaceIDForSession(sessionID)
	confirmation := &MCPConfirmation{
		ID:          model.NewID(),
		Workspace:   workspaceID,
		SessionID:   strings.TrimSpace(sessionID),
		Tool:        strings.TrimSpace(tool),
		Arguments:   args,
		Status:      "pending",
		RequestedAt: time.Now(),
		Message:     "工具需要人工确认后才能继续执行。",
		decision:    make(chan bool, 1),
	}
	if _, err := a.store.SaveMCPConfirmation(storepkg.MCPConfirmationRecord{ID: confirmation.ID, WorkspaceID: confirmation.Workspace, SessionID: confirmation.SessionID, Tool: confirmation.Tool, Arguments: confirmation.Arguments, Status: confirmation.Status, RequestedAt: confirmation.RequestedAt, Message: confirmation.Message}); err != nil {
		return err
	}
	a.confirmMu.Lock()
	a.confirmations[confirmation.ID] = confirmation
	a.confirmMu.Unlock()

	if emit != nil {
		if err := emit("tool_confirmation_required", confirmation); err != nil {
			return err
		}
	}

	var approved bool
	select {
	case approved = <-confirmation.decision:
	case <-ctx.Done():
		a.finishMCPConfirmation(confirmation.ID, "cancelled", false)
		return ctx.Err()
	case <-time.After(10 * time.Minute):
		a.finishMCPConfirmation(confirmation.ID, "expired", false)
		return fmt.Errorf("mcp tool confirmation expired: %s", tool)
	}
	if emit != nil {
		_ = emit("tool_confirmation_resolved", map[string]any{"id": confirmation.ID, "tool": tool, "approved": approved})
	}
	if !approved {
		return fmt.Errorf("mcp tool denied by user: %s", tool)
	}
	return nil
}

func (a *App) finishMCPConfirmation(id string, status string, approved bool) (MCPConfirmation, error) {
	a.confirmMu.Lock()
	defer a.confirmMu.Unlock()
	item, ok := a.confirmations[strings.TrimSpace(id)]
	if !ok {
		return MCPConfirmation{}, fmt.Errorf("mcp confirmation not found")
	}
	if item.Status != "pending" {
		return *item, nil
	}
	now := time.Now()
	item.Status = status
	item.ResolvedAt = &now
	persisted, err := a.store.ResolveMCPConfirmation(item.ID, status, approved, now)
	if err == nil {
		item.Status = persisted.Status
		item.ResolvedAt = persisted.ResolvedAt
	}
	select {
	case item.decision <- approved:
	default:
	}
	return *item, nil
}

func (a *App) listMCPConfirmations(workspaceID string) ([]storepkg.MCPConfirmationRecord, error) {
	return a.store.ListMCPConfirmations(workspaceID, true, 100)
}

func (a *App) handleListMCPConfirmations(w http.ResponseWriter, r *http.Request) {
	items, err := a.listMCPConfirmations(a.workspaceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"confirmations": items})
}

func (a *App) handleResolveMCPConfirmation(w http.ResponseWriter, r *http.Request) {
	var input MCPConfirmationResolveRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status := "denied"
	if input.Approve {
		status = "approved"
	}
	item, err := a.finishMCPConfirmation(r.PathValue("id"), status, input.Approve)
	if err != nil {
		// 重启后 pending confirmation 不再有等待中的 goroutine，但仍应能从 SQLite 中完成状态流转。
		if persisted, storeErr := a.store.ResolveMCPConfirmation(r.PathValue("id"), status, input.Approve, time.Now()); storeErr == nil {
			writeJSONResponse(w, http.StatusOK, map[string]any{"confirmation": persisted})
			return
		}

		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"confirmation": item})
}
