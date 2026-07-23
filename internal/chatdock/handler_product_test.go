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

	for _, path := range []string{"/api/setup/status", "/api/projects", "/api/model-providers", "/api/data/status", "/api/system/status"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status %d: %s", path, w.Code, w.Body.String())
		}
	}
}

func TestSystemStatusReportsAgentDockTaskCapability(t *testing.T) {
	for _, tc := range []struct {
		name       string
		contextURL string
		want       bool
	}{
		{name: "not configured"},
		{name: "configured", contextURL: "https://agentdock.example/context", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, err := NewApp(model.ServerConfig{
				Addr:                "127.0.0.1:0",
				DataDir:             t.TempDir(),
				WebDir:              "../../web",
				AgentDockContextURL: tc.contextURL,
			})
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system/status", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("system status %d: %s", w.Code, w.Body.String())
			}
			var payload storepkg.SystemStatusResponse
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.AgentDockTasksConfigured != tc.want {
				t.Fatalf("agentdock_tasks_configured = %v, want %v", payload.AgentDockTasksConfigured, tc.want)
			}
			if !payload.OK || payload.Name != "ChatDock" || payload.Database == "" {
				t.Fatalf("incomplete system status: %#v", payload)
			}
		})
	}
}

func TestProjectResourceAPIs(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader([]byte(`{"base_url":"https://example.test/v1","api_key":"secret-value","model":"demo-model","system_prompt":"全局助手","max_context_messages":8,"temperature":0.2,"hide_thinking":true}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("save global config status %d: %s", w.Code, w.Body.String())
	}
	var cfg model.PublicModelConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://example.test/v1" || cfg.Model != "demo-model" || !cfg.HasAPIKey {
		t.Fatalf("unexpected public config: %#v", cfg)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader([]byte(`{"name":"research","prompt":"只做研究总结"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create project status %d: %s", w.Code, w.Body.String())
	}
	var project model.Project
	if err := json.Unmarshal(w.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if project.ID == "" || project.Name != "research" || project.Prompt != "只做研究总结" {
		t.Fatalf("unexpected project: %#v", project)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/projects/"+project.ID+"/prompt-preview", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("prompt preview status %d: %s", w.Code, w.Body.String())
	}
	var preview model.PromptPreviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.ProjectID != project.ID || preview.Content != "全局助手\n\n只做研究总结" {
		t.Fatalf("unexpected prompt preview: %#v", preview)
	}

	r = httptest.NewRequest(http.MethodPut, "/api/projects/"+project.ID, bytes.NewReader([]byte(`{"name":"research-updated","prompt":"更新后的项目提示词"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("update project status %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if project.Name != "research-updated" || project.Prompt != "更新后的项目提示词" {
		t.Fatalf("unexpected updated project: %#v", project)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"project_id":"`+project.ID+`"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create project session status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.ProjectID != project.ID {
		t.Fatalf("session project_id = %q, want %q", session.ProjectID, project.ID)
	}

	r = httptest.NewRequest(http.MethodDelete, "/api/projects/"+project.ID, nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete project status %d: %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID, nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("get session after project delete status %d: %s", w.Code, w.Body.String())
	}
	var detachedSession model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &detachedSession); err != nil {
		t.Fatal(err)
	}
	if detachedSession.ProjectID != "" {
		t.Fatalf("deleted project should clear session project_id: %#v", detachedSession)
	}
}

func TestSetupInitPersistsAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/setup/init", bytes.NewReader([]byte(`{"base_url":"https://example.test/v1","api_key":"persisted-key","model":"daily-model","system_prompt":"每日助手"}`)))
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
	r = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status after restart %d: %s", w.Code, w.Body.String())
	}
	var status storepkg.SetupStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.NeedsSetup || status.ProjectCount != 0 || !status.HasAPIKey {
		t.Fatalf("setup state did not persist: %#v", status)
	}
}

func TestGlobalConfigIsRequestIndependent(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader([]byte(`{"base_url":"https://example.test/v1","api_key":"secret-value","model":"global-model","system_prompt":"全局助手"}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("save global config status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("global config status %d: %s", w.Code, w.Body.String())
	}
	var cfg model.PublicModelConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "global-model" {
		t.Fatalf("global config changed between requests: %#v", cfg)
	}
}
