package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatdock/internal/model"
)

func TestProviderSystemPromptEndpointUsesRealRequestCompositionAndRemovesLegacyRoute(t *testing.T) {
	runtimeContextServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"context":"AgentDock Capability Context"}`))
	}))
	defer runtimeContextServer.Close()

	app, err := NewServer(model.ServerConfig{
		DataDir:             t.TempDir(),
		AgentDockContextURL: runtimeContextServer.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	if _, err := app.store.SaveModelConfig(model.ModelConfig{
		BaseURL:      "https://provider.example/v1",
		Model:        "demo-model",
		SystemPrompt: "全局提示词",
		ContextMode:  model.ContextModeAuto,
	}); err != nil {
		t.Fatal(err)
	}
	project, err := app.store.CreateProject(model.CreateProjectRequest{Name: "研究", Prompt: "项目提示词"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := app.store.CreateSession(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := app.store.PrepareChat(model.ChatRequest{SessionID: session.ID, Message: "开始研究"}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(app.server.Handler)
	defer server.Close()

	response, err := http.Get(server.URL + "/api/sessions/" + session.ID + "/provider-system-prompt")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("provider system prompt status = %d", response.StatusCode)
	}
	var payload struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"全局提示词", "项目提示词", "AgentDock Capability Context", "ChatDock 工具资源已接入"} {
		if !strings.Contains(payload.SystemPrompt, want) {
			t.Fatalf("provider system prompt missing %q:\n%s", want, payload.SystemPrompt)
		}
	}

	legacyResponse, err := http.Get(server.URL + "/api/sessions/" + session.ID + "/system-prompt")
	if err != nil {
		t.Fatal(err)
	}
	defer legacyResponse.Body.Close()
	if legacyResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("legacy system prompt route status = %d, want 404", legacyResponse.StatusCode)
	}
}
