package model

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeModelConfigAppliesStableDefaults(t *testing.T) {
	cfg := NormalizeModelConfig(ModelConfig{
		ProviderID:         "  ",
		BaseURL:            "  ",
		Model:              " custom-model ",
		Models:             []string{" custom-model ", "other", "other", ""},
		ContextMode:        "unexpected",
		MaxContextMessages: -1,
		Temperature:        2.1,
		EmbeddingModel:     " ",
	})

	if cfg.ProviderID != "provider_default" || cfg.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("provider defaults were not applied: %#v", cfg)
	}
	if cfg.Model != "custom-model" || !reflect.DeepEqual(cfg.Models, []string{"custom-model", "other"}) {
		t.Fatalf("model names were not normalized: model=%q models=%#v", cfg.Model, cfg.Models)
	}
	if cfg.ContextMode != ContextModeAuto || cfg.MaxContextMessages != 12 || cfg.Temperature != 0.7 {
		t.Fatalf("context defaults were not applied: %#v", cfg)
	}
	if cfg.EmbeddingModel != "BAAI/bge-m3" {
		t.Fatalf("embedding model default = %q", cfg.EmbeddingModel)
	}
}

func TestNormalizeModelConfigKeepsValidBoundaryValues(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mode        string
		temperature float64
	}{
		{name: "compact zero", mode: ContextModeCompact, temperature: 0},
		{name: "expanded upper bound", mode: ContextModeExpanded, temperature: 2},
		{name: "custom", mode: ContextModeCustom, temperature: 1.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := NormalizeModelConfig(ModelConfig{ContextMode: tc.mode, Temperature: tc.temperature})
			if cfg.ContextMode != tc.mode || cfg.Temperature != tc.temperature {
				t.Fatalf("valid values changed: %#v", cfg)
			}
		})
	}
}

func TestToPublicModelConfigRedactsSecretsAndCopiesModels(t *testing.T) {
	cfg := NormalizeModelConfig(ModelConfig{
		APIKey:          " secret ",
		EmbeddingAPIKey: " embedding-secret ",
		Model:           "primary",
		Models:          []string{"primary", "secondary"},
	})
	public := ToPublicModelConfig(cfg)
	if !public.HasAPIKey || !public.HasEmbeddingAPIKey {
		t.Fatalf("secret presence flags missing: %#v", public)
	}
	public.Models[0] = "changed"
	if cfg.Models[0] != "primary" {
		t.Fatalf("public model list aliases private config: %#v", cfg.Models)
	}
	raw, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("secret")) {
		t.Fatalf("secret leaked into public config: %s", raw)
	}
}

func TestLegacyEnableThinkingConfigIsIgnored(t *testing.T) {
	var cfg ModelConfig
	if err := json.Unmarshal([]byte(`{"model":"legacy-model","enable_thinking":true,"hide_thinking":true}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "legacy-model" || !cfg.HideThinking {
		t.Fatalf("legacy config did not preserve supported fields: %#v", cfg)
	}

	publicJSON, err := json.Marshal(ToPublicModelConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(publicJSON, []byte("enable_thinking")) {
		t.Fatalf("legacy thinking switch leaked into public config: %s", publicJSON)
	}
}
