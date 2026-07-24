package chatdock

import (
	"net/http"
	"strings"

	"chatdock/internal/chatdock/model"
)

func scheduledTaskIDFromRequest(r *http.Request) string {
	return strings.TrimSpace(r.PathValue("id"))
}

func (a *App) handleListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListScheduledTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	var input model.ScheduledTaskRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.CreateScheduledTask(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleUpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	id := scheduledTaskIDFromRequest(r)
	var input model.ScheduledTaskRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.UpdateScheduledTask(id, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handlePinScheduledTask(w http.ResponseWriter, r *http.Request) {
	id := scheduledTaskIDFromRequest(r)
	var input model.PinRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.PinScheduledTask(id, input.Pinned)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleDeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	id := scheduledTaskIDFromRequest(r)
	result, err := a.store.DeleteScheduledTask(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleRunScheduledTask(w http.ResponseWriter, r *http.Request) {
	id := scheduledTaskIDFromRequest(r)
	result, err := a.executeScheduledTask(r.Context(), id, true)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleListScheduledTaskRuns(w http.ResponseWriter, r *http.Request) {
	id := scheduledTaskIDFromRequest(r)
	limit, err := parseOptionalInt(r.URL.Query().Get("limit"), 30, 1, 100, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListScheduledTaskRuns(id, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}
