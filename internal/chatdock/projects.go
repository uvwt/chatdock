package chatdock

import (
	"errors"
	"fmt"
	"net/http"

	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
)

func (a *App) handleListProjects(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var input model.CreateProjectRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	project, err := a.store.CreateProject(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, project)
}

func (a *App) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var input model.UpdateProjectRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	project, err := a.store.UpdateProject(r.PathValue("id"), input)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrProjectNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, project)
}

func (a *App) handlePinProject(w http.ResponseWriter, r *http.Request) {
	var input model.PinRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	project, err := a.store.PinProject(r.PathValue("id"), input.Pinned)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrProjectNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, project)
}

func (a *App) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	ok, err := a.store.DeleteProject(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("project not found"))
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleProjectPromptPreview(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.store.ModelConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	prompt, ok, err := a.store.ProjectPrompt(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("project not found"))
		return
	}
	writeJSONResponse(w, http.StatusOK, model.PromptPreviewResponse{ProjectID: r.PathValue("id"), Content: storepkg.BuildFinalSystemPrompt(cfg.SystemPrompt, prompt)})
}
