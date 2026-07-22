package store

import (
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestResolveFallbackModelConfigUsesProviderCredentialsAndKeepsWorkspacePolicy(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	enabled := true
	_, err = store.CreateModelProvider(ModelProviderInput{
		ID:           "backup",
		Name:         "Backup",
		BaseURL:      "https://backup.example.test/v1",
		DefaultModel: "backup-default",
		Models:       []string{"backup-default", "backup-fast"},
		Enabled:      &enabled,
		APIKeys: []ModelProviderAPIKeyInput{
			{ID: "main", Name: "主 Key", APIKey: "backup-secret", Enabled: &enabled},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	primary := model.ModelConfig{
		ProviderID:         "primary",
		BaseURL:            "https://primary.example.test/v1",
		APIKey:             "primary-secret",
		Model:              "primary-model",
		FallbackProviderID: "backup",
		FallbackModel:      "backup-fast",
		SystemPrompt:       "保持工作空间提示词",
		ContextMode:        model.ContextModeExpanded,
		MaxContextMessages: 48,
		Temperature:        0.3,
		HideThinking:       true,
	}
	fallback, err := store.ResolveFallbackModelConfig(primary)
	if err != nil {
		t.Fatal(err)
	}
	if fallback == nil {
		t.Fatal("expected resolved fallback config")
	}
	if fallback.ProviderID != "backup" || fallback.BaseURL != "https://backup.example.test/v1" || fallback.APIKey != "backup-secret" || fallback.Model != "backup-fast" {
		t.Fatalf("fallback connection was not resolved from provider: %#v", fallback)
	}
	if fallback.SystemPrompt != primary.SystemPrompt || fallback.ContextMode != primary.ContextMode || fallback.MaxContextMessages != primary.MaxContextMessages || fallback.Temperature != primary.Temperature || fallback.HideThinking != primary.HideThinking {
		t.Fatalf("workspace policy changed while resolving fallback: %#v", fallback)
	}
	if fallback.FallbackProviderID != "" || fallback.FallbackModel != "" {
		t.Fatalf("resolved fallback must not recursively contain fallback settings: %#v", fallback)
	}
}

func TestResolveFallbackModelConfigUsesProviderDefaultAndSkipsPrimaryDuplicate(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	enabled := true
	_, err = store.CreateModelProvider(ModelProviderInput{
		ID:           "same",
		Name:         "Same",
		BaseURL:      "https://same.example.test/v1",
		DefaultModel: "same-model",
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}

	fallback, err := store.ResolveFallbackModelConfig(model.ModelConfig{
		ProviderID:         "same",
		BaseURL:            "https://same.example.test/v1",
		Model:              "same-model",
		FallbackProviderID: "same",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fallback != nil {
		t.Fatalf("same primary and fallback model should be skipped: %#v", fallback)
	}
}

func TestSaveWorkspaceFallbackModelPersistsValidSelection(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	primary, err := store.SaveModelConfig(defaultWorkspaceID, model.ModelConfig{
		BaseURL: "https://primary.example.test/v1",
		APIKey:  "primary-secret",
		Model:   "primary-model",
		Models:  []string{"primary-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := store.CreateModelProvider(ModelProviderInput{
		ID:           "backup",
		Name:         "Backup",
		BaseURL:      "https://backup.example.test/v1",
		DefaultModel: "backup-default",
		Models:       []string{"backup-default", "backup-fast"},
		Enabled:      &enabled,
		APIKeys: []ModelProviderAPIKeyInput{
			{ID: "main", Name: "主 Key", APIKey: "backup-secret", Enabled: &enabled},
		},
	}); err != nil {
		t.Fatal(err)
	}

	public, err := store.SaveWorkspaceConfig(defaultWorkspaceID, model.ModelConfig{
		ProviderID:         primary.ProviderID,
		Model:              primary.Model,
		FallbackProviderID: "backup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if public.FallbackProviderID != "backup" || public.FallbackModel != "backup-default" {
		t.Fatalf("fallback selection was not normalized and persisted: %#v", public)
	}

	loaded, err := store.ModelConfig(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := store.ResolveFallbackModelConfig(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if fallback == nil || fallback.ProviderID != "backup" || fallback.Model != "backup-default" || fallback.APIKey != "backup-secret" {
		t.Fatalf("saved fallback selection did not resolve: %#v", fallback)
	}
	if err := store.DeleteModelProvider("backup"); err == nil {
		t.Fatal("provider used as workspace fallback must not be deleted")
	}
}

func TestSaveWorkspaceFallbackModelRejectsInvalidSelection(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	primary, err := store.SaveModelConfig(defaultWorkspaceID, model.ModelConfig{
		BaseURL: "https://primary.example.test/v1",
		Model:   "primary-model",
		Models:  []string{"primary-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		providerID string
		modelName  string
	}{
		{name: "missing provider", providerID: "missing", modelName: "backup-model"},
		{name: "same as primary", providerID: primary.ProviderID, modelName: primary.Model},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.SaveWorkspaceConfig(defaultWorkspaceID, model.ModelConfig{
				ProviderID:         primary.ProviderID,
				Model:              primary.Model,
				FallbackProviderID: tc.providerID,
				FallbackModel:      tc.modelName,
			})
			if err == nil {
				t.Fatal("invalid fallback selection should be rejected")
			}
		})
	}
}
