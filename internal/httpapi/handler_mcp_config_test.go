package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
)

func TestMCPConfigAPIKeepsSavedTokensOutOfBrowserResponses(t *testing.T) {
	app, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	const savedToken = "server-side-secret"
	if _, err := app.store.SaveMCPConfig(`{
      "servers": {
        "DockMini": {
          "url": "http://agentdock.test/mcp",
          "auth": {"type": "bearer", "token": "` + savedToken + `"}
        }
      }
    }`); err != nil {
		t.Fatal(err)
	}

	getResponse := requestMCPConfig(t, app, http.MethodGet, nil)
	if strings.Contains(getResponse.Content, savedToken) {
		t.Fatal("GET /api/mcp-config returned the saved token")
	}
	if !strings.Contains(getResponse.Content, `"_chatdock_token_ref": "DockMini"`) {
		t.Fatalf("GET response missing saved-token reference: %s", getResponse.Content)
	}

	preserve := model.SaveMCPConfigRequest{Content: `{
      "servers": {
        "DockMini": {
          "url": "http://changed.test/mcp",
          "auth": {"type": "bearer", "_chatdock_token_ref": "DockMini"}
        }
      }
    }`}
	preservedResponse := requestMCPConfig(t, app, http.MethodPost, preserve)
	if strings.Contains(preservedResponse.Content, savedToken) {
		t.Fatal("POST /api/mcp-config returned the preserved token")
	}
	assertStoredMCPToken(t, app, "DockMini", savedToken)

	replace := model.SaveMCPConfigRequest{Content: `{
      "servers": {
        "DockMini": {
          "url": "http://changed.test/mcp",
          "auth": {"type": "bearer", "token": "Bearer replacement-secret"}
        }
      }
    }`}
	replacedResponse := requestMCPConfig(t, app, http.MethodPost, replace)
	if strings.Contains(replacedResponse.Content, "replacement-secret") {
		t.Fatal("POST /api/mcp-config returned the replacement token")
	}
	assertStoredMCPToken(t, app, "DockMini", "replacement-secret")

	clear := model.SaveMCPConfigRequest{Content: `{
      "servers": {
        "DockMini": {
          "url": "http://changed.test/mcp",
          "auth": {"type": "bearer"}
        }
      }
    }`}
	requestMCPConfig(t, app, http.MethodPost, clear)
	assertStoredMCPToken(t, app, "DockMini", "")
}

func requestMCPConfig(t *testing.T, app *Server, method string, input any) model.MCPConfigResponse {
	t.Helper()
	var body *bytes.Reader
	if input == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, "/api/mcp-config", body)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("%s /api/mcp-config status %d: %s", method, response.Code, response.Body.String())
	}
	var payload model.MCPConfigResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertStoredMCPToken(t *testing.T, app *Server, serverName, want string) {
	t.Helper()
	raw, err := app.store.GetEffectiveMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	config, err := mcp.ParseMCPConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Servers[serverName].Auth.Token; got != want {
		t.Fatalf("stored token = %q, want %q", got, want)
	}
}
