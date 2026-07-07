package chatdock

import (
	"chatdock/internal/chatdock/model"
	"net/http"
)

func (a *App) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, model.ToPublicModelConfig(a.store.GetModelConfig()))
}

func (a *App) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var input model.ModelConfig
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	cfg, err := a.store.SaveModelConfig(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.clearQueryEmbeddingCache()
	writeJSONResponse(w, http.StatusOK, model.ToPublicModelConfig(cfg))
}
