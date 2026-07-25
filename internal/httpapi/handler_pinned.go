package httpapi

import "net/http"

func (a *Server) handleListPinned(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListPinnedFeed()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, items)
}
