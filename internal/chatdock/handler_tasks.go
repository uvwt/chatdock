package chatdock

import (
	"chatdock/internal/chatdock/model"
	"fmt"
	"net/http"
	"strings"
)

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

func (a *App) handleScheduledTaskRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/scheduled-tasks/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("scheduled task not found"))
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	if len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost {
		result, err := a.executeScheduledTask(r.Context(), a.store.ActivePrompt(), id, true)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, fmt.Errorf("scheduled task not found"))
		return
	}
	switch r.Method {
	case http.MethodPut:
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
	case http.MethodDelete:
		result, err := a.store.DeleteScheduledTask(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}
