package chatdock

import (
	"net/http"

	"chatdock/internal/chatdock/model"
)

func (a *App) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var input model.CreatePromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.store.CreatePrompt(input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func workspaceIDFromRequest(r *http.Request) string {
	return r.PathValue("id")
}

func (a *App) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromRequest(r)
	if _, err := a.store.DeletePrompt(model.SelectPromptRequest{Name: workspaceID}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleSelectWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromRequest(r)
	if _, err := a.store.SelectPrompt(model.SelectPromptRequest{Name: workspaceID}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleGetWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.store.WorkspaceConfig(workspaceIDFromRequest(r))
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
	cfg, err := a.store.SaveWorkspaceConfig(workspaceIDFromRequest(r), input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.clearQueryEmbeddingCache()
	writeJSONResponse(w, http.StatusOK, cfg)
}

func (a *App) handleWorkspacePromptPreview(w http.ResponseWriter, r *http.Request) {
	preview, err := a.store.PromptPreview(workspaceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, preview)
}
