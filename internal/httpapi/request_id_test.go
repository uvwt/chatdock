package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeRequestIDAcceptsSafeIdentifiersAndRejectsUnsafeInput(t *testing.T) {
	for _, value := range []string{"req_abc-123", "trace.id:span_1", strings.Repeat("a", maxRequestIDLength)} {
		if got := normalizeRequestID(value); got != value {
			t.Fatalf("safe request ID %q normalized to %q", value, got)
		}
	}
	for _, value := range []string{"", "contains space", "line\nbreak", "中文", strings.Repeat("a", maxRequestIDLength+1)} {
		if got := normalizeRequestID(value); got != "" {
			t.Fatalf("unsafe request ID %q normalized to %q", value, got)
		}
	}
}

func TestLogRequestReflectsSafeIDAndReplacesUnsafeID(t *testing.T) {
	handler := logRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFromRequest(r); got == "" {
			t.Fatal("request context did not receive an ID")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	safeRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	safeRequest.Header.Set("X-Request-ID", "trace-123")
	safeResponse := httptest.NewRecorder()
	handler.ServeHTTP(safeResponse, safeRequest)
	if got := safeResponse.Header().Get("X-Request-ID"); got != "trace-123" {
		t.Fatalf("safe response request ID = %q", got)
	}

	unsafeRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	unsafeRequest.Header.Set("X-Request-ID", strings.Repeat("x", maxRequestIDLength+1))
	unsafeResponse := httptest.NewRecorder()
	handler.ServeHTTP(unsafeResponse, unsafeRequest)
	got := unsafeResponse.Header().Get("X-Request-ID")
	if !strings.HasPrefix(got, "req_") || len(got) > maxRequestIDLength {
		t.Fatalf("unsafe request ID replacement = %q", got)
	}
}
