package chatdock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionRenameAndExportAPI(t *testing.T) {
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var s Session
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/sessions/"+s.ID+"/rename", bytes.NewReader([]byte(`{"title":"renamed"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("rename status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+s.ID+"/export?format=md", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("export status %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", w.Header().Get("Content-Type"))
	}
}

func TestSkillsAPI(t *testing.T) {
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader([]byte(`{"name":"写作","description":"中文写作","content":"使用短句。","enabled":true}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create skill status %d: %s", w.Code, w.Body.String())
	}
	var result SkillResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 || result.Skills[0].ID == "" {
		t.Fatalf("unexpected skill result: %#v", result)
	}

	id := result.Skills[0].ID
	r = httptest.NewRequest(http.MethodPut, "/api/skills/"+id, bytes.NewReader([]byte(`{"name":"写作","description":"中文写作","content":"使用短句。","enabled":false}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("update skill status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodDelete, "/api/skills/"+id, nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete skill status %d: %s", w.Code, w.Body.String())
	}
}

func TestScheduledTasksAPI(t *testing.T) {
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/scheduled-tasks", bytes.NewReader([]byte(`{"title":"日报","prompt":"总结今天","enabled":true,"schedule_type":"interval","interval_minutes":15}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create scheduled task status %d: %s", w.Code, w.Body.String())
	}
	var result ScheduledTaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 || result.Tasks[0].ID == "" {
		t.Fatalf("unexpected scheduled task result: %#v", result)
	}

	id := result.Tasks[0].ID
	r = httptest.NewRequest(http.MethodPut, "/api/scheduled-tasks/"+id, bytes.NewReader([]byte(`{"title":"日报","prompt":"总结今天","enabled":false,"schedule_type":"daily","time_of_day":"09:30"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("update scheduled task status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodDelete, "/api/scheduled-tasks/"+id, nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete scheduled task status %d: %s", w.Code, w.Body.String())
	}
}

func TestProductizedAPIs(t *testing.T) {
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	for _, path := range []string{"/api/setup/status", "/api/workspaces", "/api/model-providers", "/api/data/status", "/api/system/status"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status %d: %s", path, w.Code, w.Body.String())
		}
	}

	r := httptest.NewRequest(http.MethodGet, "/api/workspaces/default/prompt-preview", nil)
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("prompt preview status %d: %s", w.Code, w.Body.String())
	}
	var preview PromptPreviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Content == "" || preview.WorkspaceID != "default" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestWorkspaceResourceAPIs(t *testing.T) {
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader([]byte(`{"name":"research","system_prompt":"只做研究总结"}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create workspace status %d: %s", w.Code, w.Body.String())
	}
	var spaces WorkspaceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &spaces); err != nil {
		t.Fatal(err)
	}
	if spaces.Active != "research" || len(spaces.Workspaces) != 2 {
		t.Fatalf("unexpected workspace response: %#v", spaces)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/workspaces/research/config", bytes.NewReader([]byte(`{"base_url":"https://example.test/v1","api_key":"secret-value","model":"demo-model","system_prompt":"研究助手","max_context_messages":8,"temperature":0.2,"hide_thinking":true}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("save workspace config status %d: %s", w.Code, w.Body.String())
	}
	var cfg PublicModelConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://example.test/v1" || cfg.Model != "demo-model" || !cfg.HasAPIKey {
		t.Fatalf("unexpected public config: %#v", cfg)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/workspaces/default/select", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("select workspace status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodGet, "/api/workspaces/research/config", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("get workspace config status %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://example.test/v1" || cfg.SystemPrompt != "研究助手" {
		t.Fatalf("workspace config should be readable without switching: %#v", cfg)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/workspaces/research/select", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("reselect workspace status %d: %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("active config status %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "demo-model" {
		t.Fatalf("active config did not follow selected workspace: %#v", cfg)
	}
}

func TestSetupInitPersistsAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/setup/init", bytes.NewReader([]byte(`{"workspace_name":"daily","base_url":"https://example.test/v1","api_key":"persisted-key","model":"daily-model","system_prompt":"每日助手"}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup init status %d: %s", w.Code, w.Body.String())
	}
	if err := app.store.db.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes = restarted.routes()
	r = httptest.NewRequest(http.MethodPost, "/api/workspaces/daily/select", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("select after restart status %d: %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status after restart %d: %s", w.Code, w.Body.String())
	}
	var status SetupStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.NeedsSetup || status.ActiveWorkspace != "daily" || !status.HasAPIKey {
		t.Fatalf("setup state did not persist: %#v", status)
	}
}

func TestSPAFallbackAndBackendBoundary(t *testing.T) {
	webDir := t.TempDir()
	assetsDir := filepath.Join(webDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>ChatDock</title><div id=\"root\"></div>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('chatdock')"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: webDir})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodGet, "/workspace/demo", nil)
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ChatDock") {
		t.Fatalf("spa fallback status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "chatdock") {
		t.Fatalf("asset status %d: %s", w.Code, w.Body.String())
	}

	for _, path := range []string{"/assets/missing.js", "/api/not-found", "/mcp"} {
		r = httptest.NewRequest(http.MethodGet, path, nil)
		w = httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status %d, want 404", path, w.Code)
		}
	}
}

func TestAuthLoginWithCredential(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>ChatDock</title>"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: webDir, AuthToken: "server-token", AuthUsername: "admin", AuthCredential: "demo-value"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("auth status %d: %s", w.Code, w.Body.String())
	}
	var status AuthStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || !status.LoginEnabled || status.Username != "admin" {
		t.Fatalf("unexpected auth status: %#v", status)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`{"username":"admin","credential":"bad"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status %d, want 401", w.Code)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`{"username":"admin","credential":"demo-value"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("login status %d: %s", w.Code, w.Body.String())
	}
	var login AuthLoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if !login.OK || login.Token != "server-token" || login.Username != "admin" {
		t.Fatalf("unexpected login response: %#v", login)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.Header.Set("Authorization", "Bearer "+login.Token)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("health after login %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthLoginRequiresConfiguredCredential(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>ChatDock</title>"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: webDir, AuthToken: "server-token"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`{"credential":"server-token"}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("login without configured credential status %d, want 503", w.Code)
	}
}

func TestAuthProtectsBackendButNotEmbeddedWeb(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>ChatDock</title>"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: webDir, AuthToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodGet, "/workspace/demo", nil)
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("static route should be public, got %d", w.Code)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("api without token status %d, want 401", w.Code)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("api with token status %d: %s", w.Code, w.Body.String())
	}
}
