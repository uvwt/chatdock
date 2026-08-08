package httpapi

import (
	"fmt"
	"net/http"

	"chatdock/internal/model"
)

func (a *Server) handleContextPreview(w http.ResponseWriter, r *http.Request) {
	preview, err := a.store.ContextPreview(r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrSessionNotFound {
			status = http.StatusNotFound
		}
		writeError(w, status, fmt.Errorf("context preview failed: %w", err))
		return
	}
	writeJSONResponse(w, http.StatusOK, preview)
}
