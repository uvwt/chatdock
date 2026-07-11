package chatdock

import (
	"net/http"

	"chatdock/internal/chatdock/model"
)

func (a *App) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.store.ModelConfig(a.workspaceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, model.ToPublicModelConfig(cfg))
}

func (a *App) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var input model.ModelConfig
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	cfg, err := a.store.SaveModelConfig(a.workspaceIDFromRequest(r), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.clearQueryEmbeddingCache()
	writeJSONResponse(w, http.StatusOK, model.ToPublicModelConfig(cfg))
}
