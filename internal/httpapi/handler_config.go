package httpapi

import (
	"net/http"

	"chatdock/internal/model"
)

func (a *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.store.ModelConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, model.ToPublicModelConfig(cfg))
}

func (a *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
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
