package httpapi

import "chatdock/internal/model"

import "testing"

func TestNormalizeModelConfigDefaults(t *testing.T) {
	got := model.NormalizeModelConfig(model.ModelConfig{Temperature: 9})
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected base url: %s", got.BaseURL)
	}
	if got.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %s", got.Model)
	}
	if got.ContextMode != model.ContextModeAuto {
		t.Fatalf("unexpected context mode: %s", got.ContextMode)
	}
	if got.MaxContextMessages != 12 {
		t.Fatalf("unexpected context limit: %d", got.MaxContextMessages)
	}
	if got.Temperature != 0.7 {
		t.Fatalf("unexpected temperature: %v", got.Temperature)
	}
}

func TestNormalizeModelConfigAcceptsContextModes(t *testing.T) {
	got := model.NormalizeModelConfig(model.ModelConfig{ContextMode: model.ContextModeExpanded, MaxContextMessages: 99})
	if got.ContextMode != model.ContextModeExpanded {
		t.Fatalf("unexpected context mode: %s", got.ContextMode)
	}
	got = model.NormalizeModelConfig(model.ModelConfig{ContextMode: "bad"})
	if got.ContextMode != model.ContextModeAuto {
		t.Fatalf("unexpected fallback context mode: %s", got.ContextMode)
	}
}

func TestNormalizeModelConfigPreservesMultipleModels(t *testing.T) {
	got := model.NormalizeModelConfig(model.ModelConfig{Model: "gpt-4.1", Models: []string{"gpt-4.1", "gpt-4o-mini", "gpt-4.1"}})
	if got.Model != "gpt-4.1" {
		t.Fatalf("unexpected default model: %s", got.Model)
	}
	if len(got.Models) != 2 || got.Models[0] != "gpt-4.1" || got.Models[1] != "gpt-4o-mini" {
		t.Fatalf("unexpected models: %#v", got.Models)
	}
}
