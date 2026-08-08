package httpapi

import (
	"net/http"
	"time"
)

func (a *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, map[string]any{
		"ok":   true,
		"name": "ChatDock",
		"time": time.Now(),
	})
}
