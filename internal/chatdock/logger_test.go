package chatdock

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeLogTextRedactsSecretsAndKeepsUTF8Valid(t *testing.T) {
	input := `请求失败：Authorization: Bearer abcdefghijklmnop；api_key="sk-secret-value"；password=demo-password；https://user:pass@example.com/path；说明说明说明`
	got := sanitizeLogText(input, 90)
	if !utf8.ValidString(got) {
		t.Fatalf("sanitized log is invalid UTF-8: %q", got)
	}
	for _, secret := range []string{"abcdefghijklmnop", "sk-secret-value", "demo-password", "user:pass"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q leaked in %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redaction marker missing: %q", got)
	}
	if len([]rune(got)) > 91 {
		t.Fatalf("rune limit exceeded: %d", len([]rune(got)))
	}
}

func TestSanitizeLogValueRedactsSensitiveKeysRecursively(t *testing.T) {
	value := map[string]any{
		"request_id": "req_demo",
		"api-key":    "secret-one",
		"nested": map[string]any{
			"credential": "secret-two",
			"message":    "Bearer abcdefghijklmnop",
		},
		"items": []any{map[string]any{"password": "secret-three"}, "safe"},
	}
	got := sanitizeLogValue("payload", value).(map[string]any)
	if got["request_id"] != "req_demo" || got["api-key"] != "[REDACTED]" {
		t.Fatalf("top-level sanitized value = %#v", got)
	}
	nested := got["nested"].(map[string]any)
	if nested["credential"] != "[REDACTED]" || strings.Contains(nested["message"].(string), "abcdefghijklmnop") {
		t.Fatalf("nested sanitized value = %#v", nested)
	}
	items := got["items"].([]any)
	if items[0].(map[string]any)["password"] != "[REDACTED]" || items[1] != "safe" {
		t.Fatalf("array sanitized value = %#v", items)
	}
	if value["api-key"] != "secret-one" {
		t.Fatal("sanitization mutated the original log fields")
	}
}

func TestSensitiveLogKeyRecognitionIsConservative(t *testing.T) {
	for _, key := range []string{"Authorization", "api-key", "access token", "credential", "password", "secret"} {
		if !isSensitiveLogKey(key) {
			t.Fatalf("sensitive key %q was not recognized", key)
		}
	}
	for _, key := range []string{"token_count", "model", "request_id", "status"} {
		if isSensitiveLogKey(key) {
			t.Fatalf("ordinary key %q was incorrectly treated as sensitive", key)
		}
	}
}
