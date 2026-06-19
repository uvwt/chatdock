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
