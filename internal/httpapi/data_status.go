package httpapi

import "net/http"

func (a *Server) handleDataStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.store.DataStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}
