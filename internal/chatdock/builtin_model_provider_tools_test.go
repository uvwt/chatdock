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
		builtinToolSaveModelProvider,
		builtinToolTestModelProvider,
		builtinToolDeleteModelProvider,
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
	if len(updated.APIKeys) != 1 || updated.APIKeys[0].LastStatus != "" {
		t.Fatalf("model-list operation must not mark chat test status: %#v", updated.APIKeys)
	}
}

func TestBuiltinModelProviderTestDoesNotTreatModelListAsChatAvailability(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "listed-b"}, {"id": "listed-c"}}})
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "quota exceeded"}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	createdRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolCreateModelProvider, map[string]any{
		"name":          "False Positive Guard",
		"base_url":      modelServer.URL + "/v1",
		"api_key":       "saved-secret",
		"default_model": "initial-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := createdRaw.(map[string]any)["provider"].(store.ModelProvider)

	testRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolTestModelProvider, map[string]any{"id": provider.ID, "save_models": true})
	if err != nil {
		t.Fatal(err)
	}
	result := testRaw.(map[string]any)
	if result["ok"] != false || result["chat_test_ok"] != false || result["model_list_ok"] != true {
		t.Fatalf("model list success must not become chat availability: %#v", result)
	}
	if _, ok := result["saved"]; ok {
		t.Fatalf("failed chat test should not save models through test tool: %#v", result)
	}
	if result["operation"] != "chat_test_with_model_list" {
		t.Fatalf("unexpected operation: %#v", result)
	}

	providers, err := app.store.ListModelProviders()
	if err != nil {
		t.Fatal(err)
	}
	var updated store.ModelProvider
	for _, item := range providers {
		if item.ID == provider.ID {
			updated = item
			break
		}
	}
	if containsString(updated.Models, "listed-b") || containsString(updated.Models, "listed-c") {
		t.Fatalf("test tool should not save listed models when chat test failed: %#v", updated.Models)
	}
	if len(updated.APIKeys) != 1 || updated.APIKeys[0].LastStatus != "error" {
		t.Fatalf("chat failure should mark key test status error: %#v", updated.APIKeys)
	}
}

func TestBuiltinModelProviderSaveSupportsMultipleKeys(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}

	savedRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolSaveModelProvider, map[string]any{
		"id":            "multi-key",
		"name":          "Multi Key",
		"base_url":      "http://127.0.0.1:12345/v1",
		"default_model": "demo-model",
		"key_strategy":  "auto",
		"api_keys": []any{
			map[string]any{"id": "main", "name": "主 key", "api_key": "main-secret", "enabled": true, "priority": 2},
			map[string]any{"id": "backup", "name": "备用 key", "api_key": "backup-secret", "enabled": true, "priority": 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := savedRaw.(map[string]any)["provider"].(store.ModelProvider)
	if len(provider.APIKeys) != 2 || provider.APIKeys[0].APIKeyMasked == "" || provider.APIKeys[0].HasAPIKey != true {
		t.Fatalf("expected masked key metadata: %#v", provider)
	}
	if raw, ok := savedRaw.(map[string]any)["api_keys"]; ok && raw != nil {
		t.Fatalf("tool result should not expose raw api_keys payload: %#v", savedRaw)
	}

	cfg, ok, err := app.store.ModelProviderConfig(provider.ID)
	if err != nil || !ok {
		t.Fatalf("expected config: ok=%v err=%v", ok, err)
	}
	if cfg.APIKey != "backup-secret" {
		t.Fatalf("auto strategy should choose lowest-priority enabled key, got %q", cfg.APIKey)
	}

	updatedRaw, err := app.callBuiltinModelProviderTool(context.Background(), builtinToolSaveModelProvider, map[string]any{
		"id":              provider.ID,
		"selected_key_id": "main",
		"key_strategy":    "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := updatedRaw.(map[string]any)["provider"].(store.ModelProvider)
	if updated.SelectedKeyID != "main" || updated.KeyStrategy != "manual" {
		t.Fatalf("expected manual selected key: %#v", updated)
	}
	cfg, ok, err = app.store.ModelProviderConfig(provider.ID)
	if err != nil || !ok {
		t.Fatalf("expected config after manual switch: ok=%v err=%v", ok, err)
	}
	if cfg.APIKey != "main-secret" {
		t.Fatalf("manual strategy should use selected key, got %q", cfg.APIKey)
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
