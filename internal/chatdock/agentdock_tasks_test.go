package chatdock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestAgentDockTaskProxyUsesRuntimeAPIAndBearerToken(t *testing.T) {
	seen := make(chan *http.Request, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/runtime/tasks":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tasks": []map[string]any{{"id": "tsk_123", "title": "Build", "status": "active"}}, "count": 1})
		case "/internal/runtime/tasks/tsk_123":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "task": map[string]any{"id": "tsk_123", "steps": []map[string]any{{"id": "test", "status": "completed"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	app := newAgentTaskTestApp(t, model.ServerConfig{
		AgentDockContextURL:   upstream.URL + "/context",
		AgentDockContextToken: "agent-secret",
	})
	routes := app.routes()

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent-tasks?status=active&limit=12", nil)
	listResp := httptest.NewRecorder()
	routes.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatal(err)
	}
	if listPayload.Count != 1 {
		t.Fatalf("list count = %d", listPayload.Count)
	}
	listUpstream := <-seen
	if listUpstream.URL.Path != "/internal/runtime/tasks" || listUpstream.URL.Query().Get("status") != "active" || listUpstream.URL.Query().Get("limit") != "12" {
		t.Fatalf("unexpected list upstream request: %s", listUpstream.URL.String())
	}
	if got := listUpstream.Header.Get("Authorization"); got != "Bearer agent-secret" {
		t.Fatalf("authorization = %q", got)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent-tasks/tsk_123", nil)
	detailResp := httptest.NewRecorder()
	routes.ServeHTTP(detailResp, detailReq)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detailResp.Code, detailResp.Body.String())
	}
	detailUpstream := <-seen
	if detailUpstream.URL.Path != "/internal/runtime/tasks/tsk_123" {
		t.Fatalf("unexpected detail upstream path: %s", detailUpstream.URL.Path)
	}
}

func TestAgentDockTaskProxyValidationAndConfigurationErrors(t *testing.T) {
	app := newAgentTaskTestApp(t, model.ServerConfig{})
	routes := app.routes()

	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/api/agent-tasks?status=running", want: http.StatusBadRequest},
		{path: "/api/agent-tasks?limit=201", want: http.StatusBadRequest},
		{path: "/api/agent-tasks", want: http.StatusServiceUnavailable},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		resp := httptest.NewRecorder()
		routes.ServeHTTP(resp, req)
		if resp.Code != test.want {
			t.Fatalf("%s status = %d, want %d body=%s", test.path, resp.Code, test.want, resp.Body.String())
		}
	}
}

func TestAgentDockRuntimeURLUsesContextOriginAndPrefix(t *testing.T) {
	got, err := agentDockRuntimeURL("https://example.test/agentdock/context", "/internal/runtime/tasks", url.Values{"limit": {"20"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.test/agentdock/internal/runtime/tasks?limit=20" {
		t.Fatalf("runtime URL = %q", got)
	}
}

func newAgentTaskTestApp(t *testing.T, cfg model.ServerConfig) *App {
	t.Helper()
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>ChatDock</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Addr = "127.0.0.1:0"
	cfg.DataDir = t.TempDir()
	cfg.WebDir = webDir
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return app
}
