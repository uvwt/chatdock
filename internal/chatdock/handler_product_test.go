package chatdock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
)

func TestProductizedAPIs(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
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
	var preview storepkg.PromptPreviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Content == "" || preview.WorkspaceID != "default" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestWorkspaceResourceAPIs(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
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
	var spaces storepkg.WorkspaceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &spaces); err != nil {
		t.Fatal(err)
	}
	if spaces.Active != "default" || len(spaces.Workspaces) != 2 {
		t.Fatalf("unexpected workspace response: %#v", spaces)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/workspaces/research/config", bytes.NewReader([]byte(`{"base_url":"https://example.test/v1","api_key":"secret-value","model":"demo-model","system_prompt":"研究助手","max_context_messages":8,"temperature":0.2,"hide_thinking":true}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("save workspace config status %d: %s", w.Code, w.Body.String())
	}
	var cfg model.PublicModelConfig
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
	r.Header.Set("X-Workspace-ID", "research")
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("research scoped config status %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "demo-model" {
		t.Fatalf("research scoped config did not follow workspace header: %#v", cfg)
	}

	r = httptest.NewRequest(http.MethodDelete, "/api/workspaces/default", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete default workspace should fail with 400, got %d", w.Code)
	}

	r = httptest.NewRequest(http.MethodDelete, "/api/workspaces/research", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete workspace status %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spaces); err != nil {
		t.Fatal(err)
	}
	if spaces.Active != "default" || len(spaces.Workspaces) != 1 || spaces.Workspaces[0].ID != "default" {
		t.Fatalf("unexpected workspace response after delete: %#v", spaces)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/workspaces/research/config", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("deleted workspace config should fail with 400, got %d", w.Code)
	}
}

func TestSetupInitPersistsAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
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
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
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
	r.Header.Set("X-Workspace-ID", "daily")
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status after restart %d: %s", w.Code, w.Body.String())
	}
	var status storepkg.SetupStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.NeedsSetup || status.ActiveWorkspace != "default" || !status.HasAPIKey {
		t.Fatalf("setup state did not persist: %#v", status)
	}
}

func TestWorkspaceScopeMiddlewareLoadsDefaultAndRejectsInvalidWorkspace(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader([]byte(`{"name":"research","system_prompt":"研究"}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create workspace status %d: %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodPost, "/api/workspaces/research/config", bytes.NewReader([]byte(`{"base_url":"https://example.test/v1","api_key":"secret-value","model":"research-model","system_prompt":"研究助手"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("save research config status %d: %s", w.Code, w.Body.String())
	}

	// 显式 default 请求必须只读取 default，不能受到上一条 research 配置写入影响。
	r = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	r.Header.Set("X-Workspace-ID", "default")
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("default config status %d: %s", w.Code, w.Body.String())
	}
	var cfg model.PublicModelConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model == "research-model" {
		t.Fatalf("default scoped request used research workspace config: %#v", cfg)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	r.Header.Set("X-Workspace-ID", "missing-workspace")
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid workspace should fail with 400, got %d: %s", w.Code, w.Body.String())
	}
}
