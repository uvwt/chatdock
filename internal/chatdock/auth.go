package chatdock

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"chatdock/internal/chatdock/model"
)

func (a *App) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(a.cfg.AuthUsername)
	writeJSONResponse(w, http.StatusOK, model.AuthStatusResponse{
		Enabled:      strings.TrimSpace(a.cfg.AuthToken) != "",
		LoginEnabled: username != "" && strings.TrimSpace(a.cfg.AuthCredential) != "",
		Username:     username,
	})
}

func (a *App) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var input model.AuthLoginRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	token := strings.TrimSpace(a.cfg.AuthToken)
	if token == "" {
		writeJSONResponse(w, http.StatusOK, model.AuthLoginResponse{OK: true})
		return
	}

	username := strings.TrimSpace(a.cfg.AuthUsername)
	credential := strings.TrimSpace(a.cfg.AuthCredential)
	if username == "" || credential == "" {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("login is not configured"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(input.Username), []byte(username)) == 1 && subtle.ConstantTimeCompare([]byte(input.Credential), []byte(credential)) == 1 {
		writeJSONResponse(w, http.StatusOK, model.AuthLoginResponse{OK: true, Token: token, Username: username})
		return
	}
	writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid login"))
}
