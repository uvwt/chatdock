package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatdock/internal/model"
)

func TestAuthLoginUsesFixedLengthCredentialComparison(t *testing.T) {
	app := &Server{cfg: model.ServerConfig{
		AuthToken:      "bearer-token",
		AuthUsername:   "admin",
		AuthCredential: "correct-horse-battery-staple",
	}}

	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","credential":"correct-horse-battery-staple"}`))
	response := httptest.NewRecorder()
	app.handleAuthLogin(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("successful login status = %d, body=%s", response.Code, response.Body.String())
	}
	var success model.AuthLoginResponse
	if err := json.Unmarshal(response.Body.Bytes(), &success); err != nil {
		t.Fatal(err)
	}
	if !success.OK || success.Token != "bearer-token" || success.Username != "admin" {
		t.Fatalf("successful login response = %#v", success)
	}

	for name, body := range map[string]string{
		"short username":   `{"username":"a","credential":"correct-horse-battery-staple"}`,
		"short credential": `{"username":"admin","credential":"x"}`,
		"wrong values":     `{"username":"other","credential":"wrong-value"}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
			response := httptest.NewRecorder()
			app.handleAuthLogin(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("failed login status = %d, body=%s", response.Code, response.Body.String())
			}
		})
	}
}
