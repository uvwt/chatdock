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

func (a *App) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListPrompts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	var input model.CreatePromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.CreatePrompt(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleSelectPrompt(w http.ResponseWriter, r *http.Request) {
	var input model.SelectPromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.SelectPrompt(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}
