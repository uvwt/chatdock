package chatdock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStripThinkingContent(t *testing.T) {
	got := StripThinkingContent("hello <think>secret</think> world")
	if got != "hello  world" {
		t.Fatalf("unexpected stripped content: %q", got)
	}
	got = StripThinkingContent("hello <think>unfinished")
	if got != "hello" {
		t.Fatalf("unexpected unfinished think content: %q", got)
	}
}

func TestThinkingFilterAcrossChunks(t *testing.T) {
	f := NewThinkingFilter(true)
	parts := []string{"hel", "lo <thi", "nk>hide", "</think> ok"}
	var got string
	for _, p := range parts {
		got += f.Push(p)
	}
	got += f.Flush()
	if got != "hello  ok" {
		t.Fatalf("unexpected stream filter output: %q", got)
	}
}

func TestBuildChatMessagesLimitsContext(t *testing.T) {
	cfg := ModelConfig{SystemPrompt: "sys", MaxContextMessages: 2}
	history := []Message{{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}, {Role: "tool", Content: "ignored"}, {Role: "user", Content: "c"}}
	got := BuildChatMessages(cfg, history)
	if len(got) != 2 {
		t.Fatalf("expected system plus last valid message, got %#v", got)
	}
	if got[0]["role"] != "system" || got[1]["content"] != "c" {
		t.Fatalf("unexpected messages: %#v", got)
	}
}

func TestStreamRawMessagesEmitsIncrementalDeltas(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["stream"] != true {
			t.Fatalf("expected stream request, got %#v", body["stream"])
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server does not support flush")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, token := range []string{"一", "二", "三"} {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"` + token + `"}}]}` + "\n\n"))
			flusher.Flush()
			time.Sleep(60 * time.Millisecond)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer model.Close()

	client := NewChatClient()
	cfg := ModelConfig{BaseURL: model.URL, Model: "fake", HideThinking: true}
	var got []string
	start := time.Now()
	answer, err := client.StreamRawMessages(context.Background(), cfg, []map[string]any{{"role": "user", "content": "hi"}}, func(delta StreamDelta) error {
		got = append(got, delta.Content)
		if len(got) == 1 && time.Since(start) > 120*time.Millisecond {
			t.Fatalf("first delta arrived too late: %s", time.Since(start))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "一二三" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(got) != 3 || got[0] != "一" || got[1] != "二" || got[2] != "三" {
		t.Fatalf("expected incremental deltas, got %#v", got)
	}
}

func TestCompleteWithMCPToolsEventsStreamsWhenNoToolCall(t *testing.T) {
	requestCount := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if requestCount == 1 {
			if body["stream"] != false {
				t.Fatalf("first request should be non-stream tool decision, got %#v", body["stream"])
			}
			if _, ok := body["tools"]; !ok {
				t.Fatal("first request should include tools")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"non streamed draft"}}]}`))
			return
		}
		if body["stream"] != true {
			t.Fatalf("second request should stream final answer, got %#v", body["stream"])
		}
		if _, ok := body["tools"]; ok {
			t.Fatal("stream final answer should not include tools")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, token := range []string{"流", "式"} {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"` + token + `"}}]}` + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer model.Close()

	client := NewChatClient()
	cfg := ModelConfig{BaseURL: model.URL, Model: "fake", HideThinking: true}
	tools := []MCPTool{{Name: "noop", FullName: "noop", Description: "noop", InputSchema: map[string]any{"type": "object"}}}
	var events []string
	answer, err := client.CompleteWithMCPToolsEvents(context.Background(), cfg, []Message{{Role: "user", Content: "hi"}}, tools, func(string, map[string]any) (any, error) {
		t.Fatal("tool should not be called")
		return nil, nil
	}, func(kind string, payload any) error {
		if kind == "delta" {
			events = append(events, payload.(StreamDelta).Content)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "流式" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(events) != 2 || events[0] != "流" || events[1] != "式" {
		t.Fatalf("expected streamed events, got %#v", events)
	}
}

func TestAppendMCPToolUseHint(t *testing.T) {
	messages := []map[string]any{{"role": "system", "content": "base"}, {"role": "user", "content": "hi"}}
	out := appendMCPToolUseHint(messages, []MCPTool{{Name: "read", FullName: "agentdock__read"}})
	if len(out) != 3 {
		t.Fatalf("expected hint to be inserted, got %#v", out)
	}
	if out[1]["role"] != "system" || !strings.Contains(out[1]["content"].(string), "MCP") {
		t.Fatalf("expected MCP system hint after existing system prompt, got %#v", out)
	}
}
