package chatdock

import (
	"net/http"

	"chatdock/internal/chatdock/model"
)

func (a *App) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListWorkspaces(a.workspaceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var input model.CreateWorkspaceRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.store.CreateWorkspace(input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces(a.workspaceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func workspacePathIDFromRequest(r *http.Request) string {
	return r.PathValue("id")
}

func (a *App) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspacePathIDFromRequest(r)
	if _, err := a.store.DeleteWorkspace(model.WorkspaceIDRequest{Name: workspaceID}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces(a.workspaceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleSelectWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspacePathIDFromRequest(r)
	if err := a.store.RequireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleWorkspaceReadiness(w http.ResponseWriter, r *http.Request) {
	readiness, err := a.store.WorkspaceReadiness(workspacePathIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, readiness)
}

func (a *App) handleGetWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.store.WorkspaceConfig(workspacePathIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, cfg)
}

func (a *App) handleSaveWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	var input model.ModelConfig
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := a.store.SaveWorkspaceConfig(workspacePathIDFromRequest(r), input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.clearQueryEmbeddingCache()
	writeJSONResponse(w, http.StatusOK, cfg)
}

func (a *App) handleWorkspacePromptPreview(w http.ResponseWriter, r *http.Request) {
	preview, err := a.store.PromptPreview(workspacePathIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, preview)
}
