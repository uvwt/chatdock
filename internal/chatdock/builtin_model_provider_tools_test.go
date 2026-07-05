package chatdock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatdock/internal/chatdock/model"
	"chatdock/internal/chatdock/store"
)

func TestBuiltinModelProviderToolCRUDAndWorkspaceDefault(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}

	createdRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolCreateModelProvider, map[string]any{
		"name":          "Local Test",
		"base_url":      "http://127.0.0.1:12345/v1",
		"api_key":       "secret-value",
		"default_model": "demo-model",
		"models":        []any{"demo-model", "other-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := createdRaw.(map[string]any)
	provider := created["provider"].(store.ModelProvider)
	if provider.ID == "" || !provider.Enabled || !provider.HasAPIKey || provider.APIKeyMasked == "" {
		t.Fatalf("unexpected created provider: %#v", provider)
	}
	if raw, ok := created["api_key"]; ok && raw != "" {
		t.Fatalf("tool result should not expose api_key: %#v", created)
	}

	listedRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolListModelProviders, map[string]any{"query": "local"})
	if err != nil {
		t.Fatal(err)
	}
	listed := listedRaw.(map[string]any)
	if listed["count"].(int) != 1 {
		t.Fatalf("expected one provider in list: %#v", listed)
	}

	disabledRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolSetModelProviderEnabled, map[string]any{"id": provider.ID, "enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	disabled := disabledRaw.(map[string]any)["provider"].(store.ModelProvider)
	if disabled.Enabled {
		t.Fatalf("provider should be disabled: %#v", disabled)
	}

	_, err = app.callBuiltinModelProviderTool(context.Background(), builtinToolSetWorkspaceModelProvider, map[string]any{"provider_id": provider.ID})
	if err == nil {
		t.Fatal("disabled provider should not be selectable as workspace default")
	}

	enabledRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolSetModelProviderEnabled, map[string]any{"id": provider.ID, "enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	enabled := enabledRaw.(map[string]any)["provider"].(store.ModelProvider)
	if !enabled.Enabled {
		t.Fatalf("provider should be enabled: %#v", enabled)
	}

	workspaceRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolSetWorkspaceModelProvider, map[string]any{"provider_id": provider.ID, "model": "other-model"})
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRaw.(map[string]any)
	if workspace["provider_id"] != provider.ID || workspace["model"] != "other-model" {
		t.Fatalf("unexpected workspace switch result: %#v", workspace)
	}

	if _, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolDeleteModelProvider, map[string]any{"id": provider.ID}); err == nil {
		t.Fatal("provider used by current workspace should not be deleted")
	}
}

func TestBuiltinModelProviderToolsExposeExpectedActions(t *testing.T) {
	tools := builtinModelProviderTools()
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.FullName] = true
	}
	for _, name := range []string{
		builtinToolListModelProviders,
		builtinToolCreateModelProvider,
		builtinToolUpdateModelProvider,
		builtinToolSetModelProviderEnabled,
		builtinToolDeleteModelProvider,
		builtinToolTestModelProvider,
		builtinToolListModelProviderModels,
		builtinToolSetWorkspaceModelProvider,
	} {
		if !names[name] {
			t.Fatalf("missing builtin model provider tool %s in %#v", name, names)
		}
	}
}

func TestBuiltinModelProviderModelsCanSaveFetchedModels(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer saved-secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "fetched-a"}, {"id": "fetched-b"}}})
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	createdRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolCreateModelProvider, map[string]any{
		"name":          "Remote Test",
		"base_url":      modelServer.URL + "/v1",
		"api_key":       "saved-secret",
		"default_model": "initial-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := createdRaw.(map[string]any)["provider"].(store.ModelProvider)

	modelsRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolListModelProviderModels, map[string]any{"id": provider.ID, "save": true})
	if err != nil {
		t.Fatal(err)
	}
	modelsResult := modelsRaw.(map[string]any)
	if modelsResult["ok"] != true || modelsResult["count"] != 2 || modelsResult["saved"] != true {
		t.Fatalf("unexpected models result: %#v", modelsResult)
	}
	updated := modelsResult["provider"].(store.ModelProvider)
	if !updated.HasAPIKey || !containsString(updated.Models, "fetched-a") || !containsString(updated.Models, "fetched-b") {
		t.Fatalf("fetched models should be saved without exposing key: %#v", updated)
	}
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
