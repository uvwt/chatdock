package chatdock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAgentDockRuntimeContextSendsBearerToken(t *testing.T) {
	seenAuth := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"context":"capability context"}`))
	}))
	defer server.Close()

	text, err := fetchAgentDockRuntimeContext(context.Background(), server.URL, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if text != "capability context" {
		t.Fatalf("unexpected context: %q", text)
	}
	if got := <-seenAuth; got != "Bearer secret-token" {
		t.Fatalf("Authorization header = %q", got)
	}
}
