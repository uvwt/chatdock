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
	if got.MaxContextMessages != 12 {
		t.Fatalf("unexpected context limit: %d", got.MaxContextMessages)
	}
	if got.Temperature != 0.7 {
		t.Fatalf("unexpected temperature: %v", got.Temperature)
	}
}
