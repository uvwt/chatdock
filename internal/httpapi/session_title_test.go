package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

func TestSessionTitleGenerationReportsRepeatedEmptyContent(t *testing.T) {
	var requestCount atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer modelServer.Close()

	app, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()

	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := app.store.PrepareChat(model.ChatRequest{SessionID: session.ID, Message: "需要生成标题的问题"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.AppendAssistantMessage(session.ID, "这是回答"); err != nil {
		t.Fatal(err)
	}

	cfg := model.ModelConfig{BaseURL: modelServer.URL, Model: "title-model"}
	_, err = app.maybeGenerateSessionTitle(context.Background(), session.ID, cfg)
	if !errors.Is(err, llm.ErrEmptyModelContent) {
		t.Fatalf("repeated empty title content should be reported, got %v", err)
	}
	if got := requestCount.Load(); got != sessionTitleMaxAttempts {
		t.Fatalf("unexpected title generation attempts: got %d want %d", got, sessionTitleMaxAttempts)
	}

	stored, ok, err := app.store.GetSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("session disappeared")
	}
	if stored.Title != fallbackSessionTitle("需要生成标题的问题") {
		t.Fatalf("failed title generation should keep fallback title, got %q", stored.Title)
	}
}
