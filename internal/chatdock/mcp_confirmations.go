package chatdock

import (
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
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
	confirmation := &MCPConfirmation{
		ID:          model.NewID(),
		Workspace:   a.store.ActivePrompt(),
		SessionID:   strings.TrimSpace(sessionID),
		Tool:        strings.TrimSpace(tool),
		Arguments:   args,
		Status:      "pending",
		RequestedAt: time.Now(),
		Message:     "工具需要人工确认后才能继续执行。",
		decision:    make(chan bool, 1),
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
	select {
	case item.decision <- approved:
	default:
	}
	return *item, nil
}

func (a *App) listMCPConfirmations() []MCPConfirmation {
	workspace := a.store.ActivePrompt()
	a.confirmMu.Lock()
	defer a.confirmMu.Unlock()
	items := make([]MCPConfirmation, 0, len(a.confirmations))
	for _, item := range a.confirmations {
		if item.Workspace == workspace {
			items = append(items, *item)
		}
	}
	return items
}

func (a *App) handleListMCPConfirmations(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, map[string]any{"confirmations": a.listMCPConfirmations()})
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
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"confirmation": item})
}
