package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"chatdock/internal/chatdock/model"
)

type failingModelResponseReader struct {
	read bool
}

func (r *failingModelResponseReader) Read(buffer []byte) (int, error) {
	if r.read {
		return 0, errors.New("provider read failed")
	}
	r.read = true
	return copy(buffer, `{"choices":`), nil
}

func TestReadModelResponseBodyEnforcesDeclaredStreamingAndReadLimits(t *testing.T) {
	declared := &http.Response{ContentLength: 33, Body: io.NopCloser(strings.NewReader("small"))}
	if _, err := readModelResponseBody(declared, 32); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("declared overflow error = %v", err)
	}
	streaming := &http.Response{ContentLength: -1, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 33)))}
	if _, err := readModelResponseBody(streaming, 32); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("streaming overflow error = %v", err)
	}
	failing := &http.Response{ContentLength: -1, Body: io.NopCloser(&failingModelResponseReader{})}
	if _, err := readModelResponseBody(failing, 64); err == nil || !strings.Contains(err.Error(), "provider read failed") {
		t.Fatalf("read failure = %v", err)
	}
}

func TestModelResponseBodyLimitUsesSmallerBudgetForErrors(t *testing.T) {
	if got := modelResponseBodyLimit(&http.Response{StatusCode: http.StatusBadGateway}, 16<<20); got != maxModelErrorResponseBytes {
		t.Fatalf("error response limit = %d", got)
	}
	if got := modelResponseBodyLimit(&http.Response{StatusCode: http.StatusOK}, 16<<20); got != 16<<20 {
		t.Fatalf("success response limit = %d", got)
	}
}

func TestCompleteBoundsProviderErrorPersistedByCallers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"`+strings.Repeat("额度不足", 500)+`"}`)
	}))
	defer server.Close()

	client := NewChatClient()
	_, err := client.Complete(context.Background(), model.ModelConfig{BaseURL: server.URL, Model: "demo"}, []model.Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected provider error")
	}
	message := err.Error()
	if !strings.Contains(message, "429 Too Many Requests") || !strings.HasSuffix(message, "...") {
		t.Fatalf("unexpected provider error: %q", message)
	}
	if utf8.RuneCountInString(message) > 320 {
		t.Fatalf("provider error was not bounded: %d runes", utf8.RuneCountInString(message))
	}
}

func TestModelAPIErrorCollapsesHTMLChallenge(t *testing.T) {
	resp := &http.Response{Status: "403 Forbidden", Header: http.Header{"Content-Type": []string{"text/html"}}}
	err := modelAPIError("model api failed", resp, []byte("<!doctype html><title>Cloudflare challenge</title>"))
	if !strings.Contains(err.Error(), "Cloudflare") || strings.Contains(err.Error(), "<!doctype") {
		t.Fatalf("HTML challenge was not summarized: %v", err)
	}
}
