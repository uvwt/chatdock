package agentdock

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"chatdock/internal/model"
)

func TestRuntimeContextSendsBearerToken(t *testing.T) {
	seenAuth := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"context":"capability context"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-token")
	if got := client.RuntimeContext(context.Background()); got != "capability context" {
		t.Fatalf("runtime context = %q", got)
	}
	if got := <-seenAuth; got != "Bearer secret-token" {
		t.Fatalf("Authorization header = %q", got)
	}
}

func TestRuntimeURLUsesContextOriginAndPrefix(t *testing.T) {
	got, err := runtimeURL("https://example.test/agentdock/context", "/internal/runtime/tasks", url.Values{"limit": {"20"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.test/agentdock/internal/runtime/tasks?limit=20" {
		t.Fatalf("runtime URL = %q", got)
	}
}

func TestSessionTaskIDFromDirectTaskManageEvent(t *testing.T) {
	event := model.MessageEvent{Details: map[string]any{
		"tool":      "DockMini.task_manage",
		"arguments": map[string]any{"action": "checkpoint", "task_id": "tsk_direct"},
		"result":    map[string]any{"task_id": "tsk_direct"},
	}}
	if got := SessionTaskIDFromEvent(event); got != "tsk_direct" {
		t.Fatalf("direct task id = %q", got)
	}
	event.Details["arguments"] = map[string]any{"action": "get", "task_id": "tsk_read_only"}
	if got := SessionTaskIDFromEvent(event); got != "" {
		t.Fatalf("read-only task lookup must not bind session, got %q", got)
	}
}

func TestDecodeBoundedJSONAcceptsOneCompleteValue(t *testing.T) {
	var payload map[string]any
	if err := decodeBoundedJSON(strings.NewReader(`{"ok":true}`), 32, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true {
		t.Fatalf("payload = %#v", payload)
	}
}

type failingJSONReader struct{ read bool }

func (r *failingJSONReader) Read(buffer []byte) (int, error) {
	if r.read {
		return 0, errors.New("upstream read failed")
	}
	r.read = true
	return copy(buffer, `{"ok":`), nil
}

func TestDecodeBoundedJSONRejectsUnsafeResponses(t *testing.T) {
	cases := map[string]struct {
		reader io.Reader
		limit  int64
		want   string
	}{
		"overflow after valid JSON": {reader: strings.NewReader(`{"ok":true}` + strings.Repeat(" ", 24)), limit: 24, want: "exceeds 24 bytes"},
		"trailing JSON value":       {reader: strings.NewReader(`{"ok":true}{"extra":true}`), limit: 64, want: "invalid character"},
		"read failure":              {reader: &failingJSONReader{}, limit: 64, want: "upstream read failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var payload map[string]any
			if err := decodeBoundedJSON(tc.reader, tc.limit, &payload); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("decode error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestUpstreamsRejectOversizedJSONResponses(t *testing.T) {
	body := `{"context":"ok"}` + strings.Repeat(" ", taskResponseLimit+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	if _, err := client.fetchContext(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("context overflow error = %v", err)
	}
	if _, _, err := client.RequestTaskJSON(context.Background(), http.MethodGet, "/internal/runtime/tasks", nil); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("task overflow error = %v", err)
	}
}

func TestRequestTaskJSONPreservesPayloadAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad task"})
	}))
	defer server.Close()

	payload, status, err := NewClient(server.URL+"/context", "").RequestTaskJSON(context.Background(), http.MethodGet, "/internal/runtime/tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusBadRequest || payload["error"] != "bad task" {
		t.Fatalf("status=%d payload=%#v", status, payload)
	}
}
