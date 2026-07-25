package httpapi

import (
	"fmt"
	"net/http"
)

func (a *Server) handleSessionSystemPrompt(w http.ResponseWriter, r *http.Request) {
	prompt, err := a.store.SessionSystemPrompt(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("session system prompt failed: %w", err))
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"system_prompt": prompt})
}
