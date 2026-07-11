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

	createdRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{
		"name":          "Local Test",
		"base_url":      "http://127.0.0.1:12345/v1",
		"default_model": "demo-model",
		"models":        []any{"demo-model", "other-model"},
		"api_keys": []any{
			map[string]any{"id": "main", "name": "主 key", "api_key": "secret-value", "enabled": true, "priority": 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := createdRaw.(map[string]any)
	provider := created["provider"].(store.ModelProvider)
	if provider.ID == "" || !provider.Enabled || !provider.HasAPIKey || provider.APIKeyMasked == "" {
		t.Fatalf("unexpected created provider: %#v", provider)
	}
	if raw, ok := created["api_keys"]; ok && raw != nil {
		t.Fatalf("tool result should not expose api_keys: %#v", created)
	}

	listedRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolListModelProviders, map[string]any{"query": "local"})
	if err != nil {
		t.Fatal(err)
	}
	listed := listedRaw.(map[string]any)
	if listed["count"].(int) != 1 {
		t.Fatalf("expected one provider in list: %#v", listed)
	}

	disabledRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{"id": provider.ID, "enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	disabled := disabledRaw.(map[string]any)["provider"].(store.ModelProvider)
	if disabled.Enabled {
		t.Fatalf("provider should be disabled: %#v", disabled)
	}

	if _, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{"id": provider.ID, "set_as_workspace_default": true}); err == nil {
		t.Fatal("disabled provider should not be selectable as workspace default")
	}

	enabledRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{
		"id":                       provider.ID,
		"enabled":                  true,
		"set_as_workspace_default": true,
		"workspace_model":          "other-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	enabledResult := enabledRaw.(map[string]any)
	enabled := enabledResult["provider"].(store.ModelProvider)
	if !enabled.Enabled {
		t.Fatalf("provider should be enabled: %#v", enabled)
	}
	workspace := enabledResult["workspace"].(map[string]any)
	if workspace["provider_id"] != provider.ID || workspace["model"] != "other-model" {
		t.Fatalf("unexpected workspace switch result: %#v", workspace)
	}

	if _, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolDeleteModelProvider, map[string]any{"id": provider.ID}); err == nil {
		t.Fatal("provider used by current workspace should not be deleted")
	}
}

func TestBuiltinModelProviderToolsExposeExpectedActions(t *testing.T) {
	tools := builtinModelProviderTools()
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.FullName] = true
	}
	expected := []string{
		builtinToolListModelProviders,
		builtinToolSaveModelProvider,
		builtinToolTestModelProvider,
		builtinToolDeleteModelProvider,
	}
	if len(names) != len(expected) {
		t.Fatalf("unexpected builtin model provider tools: %#v", names)
	}
	for _, name := range expected {
		if !names[name] {
			t.Fatalf("missing builtin model provider tool %s in %#v", name, names)
		}
	}
}

func TestBuiltinModelProviderTestReturnsCandidateCatalogWithoutSaving(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer saved-secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "fetched-a"}, {"id": "fetched-b"}}})
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "OK"}}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	createdRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{
		"name":          "Remote Test",
		"base_url":      modelServer.URL + "/v1",
		"default_model": "initial-model",
		"api_keys": []any{
			map[string]any{"id": "main", "name": "主 key", "api_key": "saved-secret", "enabled": true, "priority": 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := createdRaw.(map[string]any)["provider"].(store.ModelProvider)

	testRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolTestModelProvider, map[string]any{"id": provider.ID, "fetch_models": true})
	if err != nil {
		t.Fatal(err)
	}
	result := testRaw.(map[string]any)
	if result["ok"] != true || result["chat_test_ok"] != true || result["model_list_ok"] != true || result["count"] != 2 || result["operation"] != "chat_test_with_model_list" {
		t.Fatalf("unexpected provider test result: %#v", result)
	}
	if _, ok := result["saved"]; ok {
		t.Fatalf("provider test must not auto-save candidate models: %#v", result)
	}
	candidates, _ := result["candidate_models"].([]string)
	if !containsString(candidates, "fetched-a") || !containsString(candidates, "fetched-b") {
		t.Fatalf("candidate models should be returned: %#v", result)
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
	if containsString(updated.Models, "fetched-a") || containsString(updated.Models, "fetched-b") {
		t.Fatalf("candidate catalog must not be saved automatically: %#v", updated.Models)
	}
	if len(updated.APIKeys) != 1 || updated.APIKeys[0].LastStatus != "ok" {
		t.Fatalf("successful chat test should mark key status ok: %#v", updated.APIKeys)
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
	createdRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{
		"name":          "False Positive Guard",
		"base_url":      modelServer.URL + "/v1",
		"default_model": "initial-model",
		"api_keys": []any{
			map[string]any{"id": "main", "name": "主 key", "api_key": "saved-secret", "enabled": true, "priority": 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := createdRaw.(map[string]any)["provider"].(store.ModelProvider)

	testRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolTestModelProvider, map[string]any{"id": provider.ID, "fetch_models": true})
	if err != nil {
		t.Fatal(err)
	}
	result := testRaw.(map[string]any)
	if result["ok"] != false || result["chat_test_ok"] != false || result["model_list_ok"] != true {
		t.Fatalf("candidate catalog success must not become chat availability: %#v", result)
	}
	if _, ok := result["saved"]; ok {
		t.Fatalf("test tool must not auto-save candidate models: %#v", result)
	}
	if result["operation"] != "chat_test_with_model_list" {
		t.Fatalf("unexpected operation: %#v", result)
	}
	candidates, _ := result["candidate_models"].([]string)
	if !containsString(candidates, "listed-b") || !containsString(candidates, "listed-c") {
		t.Fatalf("candidate models should be returned alongside failed chat test: %#v", result)
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
		t.Fatalf("test tool must not auto-save candidate models: %#v", updated.Models)
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

	savedRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{
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

	updatedRaw, err := app.callBuiltinModelProviderTool(context.Background(), "default", builtinToolSaveModelProvider, map[string]any{
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

func TestModelProviderInputRequiresStringArrayModels(t *testing.T) {
	base := map[string]any{
		"name":          "Strict Models",
		"base_url":      "http://127.0.0.1:12345/v1",
		"default_model": "demo-model",
	}

	for name, invalid := range map[string]any{
		"string":    "demo-model,other-model",
		"nonstring": []any{"demo-model", 42},
	} {
		t.Run(name, func(t *testing.T) {
			args := map[string]any{}
			for key, value := range base {
				args[key] = value
			}
			args["models"] = invalid
			if _, err := modelProviderInputFromArgs(args, nil); err == nil {
				t.Fatalf("expected invalid models to fail: %#v", invalid)
			}
		})
	}

	args := map[string]any{}
	for key, value := range base {
		args[key] = value
	}
	args["models"] = []any{" demo-model ", "other-model", "demo-model"}
	input, err := modelProviderInputFromArgs(args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Models) != 2 || input.Models[0] != "demo-model" || input.Models[1] != "other-model" {
		t.Fatalf("unexpected normalized models: %#v", input.Models)
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
