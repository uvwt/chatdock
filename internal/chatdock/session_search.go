package chatdock

import (
	"fmt"
	"net/http"
)

func (a *App) handleSearchSessions(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.SearchSessions(r.URL.Query().Get("q"), 80)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("search sessions failed: %w", err))
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"sessions": items})
}
