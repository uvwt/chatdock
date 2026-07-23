package chatdock

import (
	"fmt"
	"net/http"
)

func (a *App) handleSearchSessions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, err := parseOptionalInt(query.Get("limit"), 30, 1, 100, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, nextCursor, hasMore, err := a.store.SearchSessionPage(query.Get("q"), query.Get("cursor"), limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("search sessions failed: %w", err))
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"sessions": items, "next_cursor": nextCursor, "has_more": hasMore})
}
