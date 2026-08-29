package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"chatdock/internal/model"
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

func TestModelRetryDelayClassifiesTransientFailures(t *testing.T) {
	tests := map[string]struct {
		err       error
		fallback  time.Duration
		wantDelay time.Duration
		wantRetry bool
	}{
		"unexpected eof": {
			err:       io.ErrUnexpectedEOF,
			fallback:  500 * time.Millisecond,
			wantDelay: 500 * time.Millisecond,
			wantRetry: true,
		},
		"service unavailable": {
			err: modelAPIError("model api failed", &http.Response{
				Status:     "503 Service Unavailable",
				StatusCode: http.StatusServiceUnavailable,
				Header:     make(http.Header),
			}, []byte(`{"error":"busy"}`)),
			fallback:  500 * time.Millisecond,
			wantDelay: 500 * time.Millisecond,
			wantRetry: true,
		},
		"retry after": {
			err: modelAPIError("model api failed", &http.Response{
				Status:     "429 Too Many Requests",
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"2"}},
			}, []byte(`{"error":"rate limited"}`)),
			fallback:  500 * time.Millisecond,
			wantDelay: 2 * time.Second,
			wantRetry: true,
		},
		"bad request": {
			err: modelAPIError("model api failed", &http.Response{
				Status:     "400 Bad Request",
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
			}, []byte(`{"error":"invalid arguments"}`)),
			fallback:  500 * time.Millisecond,
			wantRetry: false,
		},
		"context overflow wrapped as service unavailable": {
			err: modelAPIError("model api failed", &http.Response{
				Status:     "503 Service Unavailable",
				StatusCode: http.StatusServiceUnavailable,
				Header:     make(http.Header),
			}, []byte(`{"error":{"code":"context_length_exceeded","message":"maximum context length exceeded"}}`)),
			fallback:  500 * time.Millisecond,
			wantRetry: false,
		},
		"canceled": {
			err:       context.Canceled,
			fallback:  500 * time.Millisecond,
			wantRetry: false,
		},
		"protocol error": {
			err:       errors.New("expected JSON object for tool arguments"),
			fallback:  500 * time.Millisecond,
			wantRetry: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			delay, retry := ModelRetryDelay(tc.err, tc.fallback)
			if retry != tc.wantRetry || delay != tc.wantDelay {
				t.Fatalf("ModelRetryDelay() = (%v, %v), want (%v, %v)", delay, retry, tc.wantDelay, tc.wantRetry)
			}
		})
	}
}

func TestModelAPIErrorKeepsContextClassificationBeyondSummaryLimit(t *testing.T) {
	body := []byte(strings.Repeat("gateway metadata ", 40) + `{"error":{"code":"context_too_large","message":"input exceeds the context window"}}`)
	err := modelAPIError("model api failed", &http.Response{
		Status:     "503 Service Unavailable",
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header),
	}, body)

	if !IsContextTooLargeModelError(err) {
		t.Fatalf("full provider body context overflow was not classified: %v", err)
	}
	if delay, retry := ModelRetryDelay(err, 500*time.Millisecond); retry || delay != 0 {
		t.Fatalf("context overflow must not retry on the same model: delay=%v retry=%v", delay, retry)
	}
	if !IsRetryableModelError(err) {
		t.Fatal("context overflow should remain eligible for fallback model routing")
	}
	if !strings.Contains(err.Error(), "context_too_large") {
		t.Fatalf("bounded error text lost the structured context marker: %v", err)
	}
}

func TestModelRetryDelaySkipsLongRetryAfterButKeepsFallbackEligible(t *testing.T) {
	err := modelAPIError("model api failed", &http.Response{
		Status:     "429 Too Many Requests",
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"30"}},
	}, []byte(`{"error":"rate limited"}`))

	if delay, retry := ModelRetryDelay(err, 500*time.Millisecond); retry || delay != 0 {
		t.Fatalf("long Retry-After must skip same-model retry: delay=%v retry=%v", delay, retry)
	}
	if !IsRetryableModelError(err) {
		t.Fatal("long Retry-After must remain eligible for fallback model routing")
	}
}
