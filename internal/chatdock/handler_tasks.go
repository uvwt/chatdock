package chatdock

import (
	"chatdock/internal/chatdock/model"
	"net/http"
	"strconv"
	"strings"
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
	result, err := a.executeScheduledTask(r.Context(), a.store.ActiveWorkspace(), id, true)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleListScheduledTaskRuns(w http.ResponseWriter, r *http.Request) {
	id := scheduledTaskIDFromRequest(r)
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	result, err := a.store.ListScheduledTaskRuns(id, limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}
