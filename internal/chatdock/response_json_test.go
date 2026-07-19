package chatdock

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingJSONReader struct {
	read bool
}

func (r *failingJSONReader) Read(buffer []byte) (int, error) {
	if r.read {
		return 0, errors.New("upstream read failed")
	}
	r.read = true
	return copy(buffer, `{"ok":`), nil
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

func TestDecodeBoundedJSONRejectsOverflowTrailingValuesAndReadErrors(t *testing.T) {
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

func TestAgentDockUpstreamsRejectOversizedJSONResponses(t *testing.T) {
	body := `{"context":"ok"}` + strings.Repeat(" ", agentDockTaskResponseLimit+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	if _, err := fetchAgentDockRuntimeContext(context.Background(), server.URL, ""); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("context overflow error = %v", err)
	}
	if _, _, err := requestAgentDockRuntimeJSON(context.Background(), server.URL, "", http.MethodGet, "/internal/runtime/tasks", nil); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("task overflow error = %v", err)
	}
}
