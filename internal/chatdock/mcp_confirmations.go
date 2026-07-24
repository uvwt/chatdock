package chatdock

import (
	"errors"
	"net/http"
	"time"

	"chatdock/internal/chatdock/toolapproval"
)

type MCPConfirmationResolveRequest struct {
	Approve bool `json:"approve"`
}

func (a *App) handleListMCPConfirmations(w http.ResponseWriter, r *http.Request) {
	items, err := a.approvals.List()
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
	item, err := a.approvals.Finish(r.PathValue("id"), status, input.Approve)
	if err == nil {
		writeJSONResponse(w, http.StatusOK, map[string]any{"confirmation": item})
		return
	}
	if !errors.Is(err, toolapproval.ErrNotActive) {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 重启后 pending confirmation 不再有等待中的 goroutine，但仍应能从 SQLite 中完成状态流转。
	persisted, storeErr := a.store.ResolveMCPConfirmation(r.PathValue("id"), status, input.Approve, time.Now())
	if storeErr != nil {
		writeError(w, http.StatusNotFound, storeErr)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"confirmation": persisted})
}
