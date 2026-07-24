package httpapi

import (
	"net/http"

	storepkg "chatdock/internal/store"
)

func (a *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.store.SetupStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}

func (a *Server) handleSetupInit(w http.ResponseWriter, r *http.Request) {
	var input storepkg.SetupInitRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, err := a.store.InitializeSetup(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}
