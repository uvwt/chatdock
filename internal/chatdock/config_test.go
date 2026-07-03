package chatdock

import "testing"

func TestNormalizeModelConfigDefaults(t *testing.T) {
	got := NormalizeModelConfig(ModelConfig{Temperature: 9})
	if got.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected base url: %s", got.BaseURL)
	}
	if got.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected model: %s", got.Model)
	}
	if got.ContextMode != ContextModeAuto {
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
	got := NormalizeModelConfig(ModelConfig{ContextMode: ContextModeExpanded, MaxContextMessages: 99})
	if got.ContextMode != ContextModeExpanded {
		t.Fatalf("unexpected context mode: %s", got.ContextMode)
	}
	got = NormalizeModelConfig(ModelConfig{ContextMode: "bad"})
	if got.ContextMode != ContextModeAuto {
		t.Fatalf("unexpected fallback context mode: %s", got.ContextMode)
	}
}
