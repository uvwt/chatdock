package chatdock

import "net/http"

func (a *App) handleDataStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.store.DataStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}
