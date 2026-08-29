package llm

import "testing"

func TestIsContextTooLargeErrorText(t *testing.T) {
	for _, raw := range []string{
		`{"error":{"code":"context_too_large"}}`,
		`maximum context length is 128000 tokens`,
		`Your input exceeds the context window of this model`,
		`prompt is too long`,
	} {
		if !IsContextTooLargeErrorText(raw) {
			t.Fatalf("expected context overflow match: %q", raw)
		}
	}
}

func TestIsContextTooLargeErrorTextDoesNotMatchRateLimitTokenMessage(t *testing.T) {
	raw := `429 rate limit exceeded: too many tokens per minute`
	if IsContextTooLargeErrorText(raw) {
		t.Fatalf("rate-limit token message must not be classified as context overflow: %q", raw)
	}
}
