package chatdock

import (
	"bytes"
	"chatdock/internal/chatdock/model"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatAPIWritesSingleJSONResponse(t *testing.T) {
	seenPath := make(chan string, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seenPath <- r.URL.Path:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"模型回答"}}]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveModelConfig(model.ModelConfig{BaseURL: modelServer.URL, Model: "demo", SystemPrompt: "测试助手"}); err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create session status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader([]byte(`{"session_id":"`+session.ID+`","message":"你好"}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("chat status %d: %s", w.Code, w.Body.String())
	}

	decoder := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
	var result model.ChatResponse
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode chat response: %v; body=%q", err, w.Body.String())
	}
	if decoder.More() {
		t.Fatalf("response contains more than one JSON value: %q", w.Body.String())
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("response has trailing JSON/content: err=%v body=%q", err, w.Body.String())
	}
	if got := <-seenPath; got != "/chat/completions" {
		t.Fatalf("unexpected model path: %s", got)
	}
	if result.Answer != "模型回答" || result.Session == nil || len(result.Session.Messages) != 2 {
		t.Fatalf("unexpected chat response: %#v", result)
	}
}
