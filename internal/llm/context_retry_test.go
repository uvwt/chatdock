package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"chatdock/internal/model"
)

func TestCompleteWithMCPToolsRetriesContextErrorWithAggressiveCompression(t *testing.T) {
	var requests atomic.Int32
	var firstBody, secondBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if requests.Add(1) == 1 {
			firstBody = body
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"message":"context length exceeded"}}`)
			return
		}
		secondBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"压缩后成功"}}]}`)
	}))
	defer server.Close()

	old := strings.Repeat("old context ", 180)
	cfg := model.ModelConfig{BaseURL: server.URL, Model: "demo", ContextWindowTokens: 32 * 1024, OutputReserveTokens: 4 * 1024}
	history := []model.Message{
		{Role: "user", Content: old},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "recent question"},
		{Role: "assistant", Content: "recent answer"},
		{Role: "user", Content: "current question"},
	}
	answer, err := NewChatClient().CompleteWithMCPToolsEvents(context.Background(), cfg, history, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "压缩后成功" || requests.Load() != 2 {
		t.Fatalf("answer=%q requests=%d", answer, requests.Load())
	}
	firstMessages, _ := firstBody["messages"].([]any)
	if len(firstMessages) != 5 {
		t.Fatalf("unexpected first request messages: %#v", firstBody["messages"])
	}
	secondMessages, _ := secondBody["messages"].([]any)
	if len(secondMessages) >= len(firstMessages) {
		t.Fatalf("emergency retry did not tighten context: first=%#v second=%#v", firstBody["messages"], secondBody["messages"])
	}
	if len(secondMessages) < 2 {
		t.Fatalf("emergency retry lost required context: %#v", secondBody["messages"])
	}
	if content := fmt.Sprint(secondMessages[len(secondMessages)-1].(map[string]any)["content"]); content != "current question" {
		t.Fatalf("emergency retry changed current question: %#v", secondBody["messages"])
	}
	if summary := fmt.Sprint(secondMessages[0].(map[string]any)["content"]); !strings.Contains(summary, "# 早期会话摘要") {
		t.Fatalf("emergency retry summary missing: %#v", secondBody["messages"])
	}
}
