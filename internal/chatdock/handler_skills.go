package chatdock

import (
	"chatdock/internal/chatdock/model"
	"fmt"
	"net/http"
	"strings"
)

func (a *App) handleListSkills(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var input model.SaveSkillRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.CreateSkill(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleSkillRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	id := strings.Trim(path, "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, fmt.Errorf("skill not found"))
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input model.SaveSkillRequest
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.UpdateSkill(id, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	case http.MethodDelete:
		result, err := a.store.DeleteSkill(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}
