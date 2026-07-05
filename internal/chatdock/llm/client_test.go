package llm

import (
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	"context"
	"encoding/json"
	"fmt"
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

func TestBuildChatMessagesCustomModeLimitsContext(t *testing.T) {
	cfg := model.ModelConfig{SystemPrompt: "sys", ContextMode: model.ContextModeCustom, MaxContextMessages: 2}
	history := []model.Message{{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}, {Role: "tool", Content: "ignored"}, {Role: "user", Content: "c"}}
	got := BuildChatMessages(cfg, history)
	if len(got) != 3 {
		t.Fatalf("expected system plus last two valid messages, got %#v", got)
	}
	if got[0]["role"] != "system" || got[1]["content"] != "b" || got[2]["content"] != "c" {
		t.Fatalf("unexpected messages: %#v", got)
	}
}

func TestBuildChatMessagesHoistsRuntimeSystemContext(t *testing.T) {
	cfg := model.ModelConfig{SystemPrompt: "base system", ContextMode: model.ContextModeCustom, MaxContextMessages: 1}
	history := []model.Message{
		{Role: "user", Content: "older user"},
		{Role: "assistant", Content: "older assistant"},
		{Role: "system", Content: "AgentDock Capability Context"},
		{Role: "user", Content: "latest user"},
	}
	got := BuildChatMessages(cfg, history)
	if len(got) != 3 {
		t.Fatalf("expected base system, runtime system, and latest user, got %#v", got)
	}
	if got[0]["content"] != "base system" || got[1]["content"] != "AgentDock Capability Context" || got[2]["content"] != "latest user" {
		t.Fatalf("runtime system context was not preserved before user message: %#v", got)
	}
}

func TestBuildChatMessagesAutoSummarizesEarlierContext(t *testing.T) {
	cfg := model.ModelConfig{SystemPrompt: "sys", ContextMode: model.ContextModeAuto}
	history := make([]model.Message, 0, 14)
	for i := 1; i <= 14; i++ {
		history = append(history, model.Message{Role: "user", Content: fmt.Sprintf("message-%02d", i)})
	}
	got := BuildChatMessages(cfg, history)
	if len(got) != 14 {
		t.Fatalf("expected system, summary, and 12 recent messages, got %#v", got)
	}
	if !strings.Contains(got[1]["content"], "早期会话摘要") || !strings.Contains(got[1]["content"], "message-02") {
		t.Fatalf("expected earlier context summary, got %#v", got[1])
	}
	if got[2]["content"] != "message-03" || got[len(got)-1]["content"] != "message-14" {
		t.Fatalf("recent messages were not preserved: %#v", got)
	}
}

func TestStreamRawMessagesEmitsIncrementalDeltas(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer modelServer.Close()

	client := NewChatClient()
	cfg := model.ModelConfig{BaseURL: modelServer.URL, Model: "fake", HideThinking: true}
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
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["stream"] != true {
			t.Fatalf("tool request should stream, got %#v", body["stream"])
		}
		if _, ok := body["tools"]; !ok {
			t.Fatal("streaming request should still include tools")
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
	defer modelServer.Close()

	client := NewChatClient()
	cfg := model.ModelConfig{BaseURL: modelServer.URL, Model: "fake", HideThinking: true}
	tools := []mcp.MCPTool{{Name: "noop", FullName: "noop", Description: "noop", InputSchema: map[string]any{"type": "object"}}}
	var events []string
	answer, err := client.CompleteWithMCPToolsEvents(context.Background(), cfg, []model.Message{{Role: "user", Content: "hi"}}, tools, func(string, map[string]any) (any, error) {
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
	if requestCount != 1 {
		t.Fatalf("plain tool-aware response should complete in one streaming request, got %d", requestCount)
	}
	if len(events) != 2 || events[0] != "流" || events[1] != "式" {
		t.Fatalf("expected streamed events, got %#v", events)
	}
}

func TestCompleteWithMCPToolsEventsAllowsMultipleToolRoundsBeforeStreaming(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["stream"] != true {
			t.Fatalf("request %d should stream, got %#v", requestCount, body["stream"])
		}
		if _, ok := body["tools"]; !ok {
			t.Fatalf("request %d should keep tools available", requestCount)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		send := func(data string) {
			_, _ = w.Write([]byte("data: " + data + "\n\n"))
			flusher.Flush()
		}
		switch requestCount {
		case 1:
			send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"first_tool","arguments":"{\"step\":"}}]}}]}`)
			send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}}]}`)
		case 2:
			messages := body["messages"].([]any)
			last := messages[len(messages)-1].(map[string]any)
			if last["role"] != "tool" || last["name"] != "first_tool" {
				t.Fatalf("second request should include first tool result, got %#v", last)
			}
			send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"second_tool","arguments":"{\"step\":"}}]}}]}`)
			send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"2}"}}]}}]}`)
		case 3:
			messages := body["messages"].([]any)
			last := messages[len(messages)-1].(map[string]any)
			if last["role"] != "tool" || last["name"] != "second_tool" {
				t.Fatalf("third request should include second tool result, got %#v", last)
			}
			send(`{"choices":[{"delta":{"content":"完成"}}]}`)
		default:
			t.Fatalf("unexpected request %d", requestCount)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer modelServer.Close()

	client := NewChatClient()
	cfg := model.ModelConfig{BaseURL: modelServer.URL, Model: "fake", HideThinking: true}
	tools := []mcp.MCPTool{{Name: "noop", FullName: "noop", Description: "noop", InputSchema: map[string]any{"type": "object"}}}
	var called []string
	answer, err := client.CompleteWithMCPToolsEvents(context.Background(), cfg, []model.Message{{Role: "user", Content: "hi"}}, tools, func(name string, args map[string]any) (any, error) {
		called = append(called, name)
		return map[string]any{"name": name, "args": args}, nil
	}, func(kind string, payload any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if answer != "完成" {
		t.Fatalf("unexpected streamed answer: %q", answer)
	}
	if requestCount != 3 {
		t.Fatalf("expected three streaming model requests, got %d", requestCount)
	}
	if len(called) != 2 || called[0] != "first_tool" || called[1] != "second_tool" {
		t.Fatalf("expected two tool rounds, got %#v", called)
	}
}

func TestCompleteWithMCPToolsEventsInjectsToolImageAsVisionMessage(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		send := func(data string) {
			_, _ = w.Write([]byte("data: " + data + "\n\n"))
			flusher.Flush()
		}
		switch requestCount {
		case 1:
			send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_img","type":"function","function":{"name":"image_loader","arguments":"{\"url\":\"https://example.com/cat.png\"}"}}]}}]}`)
		case 2:
			messages := body["messages"].([]any)
			if len(messages) < 2 {
				t.Fatalf("expected tool and vision messages, got %#v", messages)
			}
			toolMessage := messages[len(messages)-2].(map[string]any)
			if toolMessage["role"] != "tool" || toolMessage["name"] != "image_loader" {
				t.Fatalf("expected sanitized tool message before vision message, got %#v", toolMessage)
			}
			toolContent, _ := toolMessage["content"].(string)
			if strings.Contains(toolContent, "_chatdock_model_content") || !strings.Contains(toolContent, "image_url") {
				t.Fatalf("tool content should be sanitized metadata, got %s", toolContent)
			}
			visionMessage := messages[len(messages)-1].(map[string]any)
			if visionMessage["role"] != "user" {
				t.Fatalf("vision content should be sent as user message, got %#v", visionMessage)
			}
			blocks := visionMessage["content"].([]any)
			if len(blocks) != 2 {
				t.Fatalf("expected text + image blocks, got %#v", blocks)
			}
			imageBlock := blocks[1].(map[string]any)
			imageURL := imageBlock["image_url"].(map[string]any)
			if imageBlock["type"] != "image_url" || imageURL["url"] != "https://example.com/cat.png" {
				t.Fatalf("expected image_url content block, got %#v", imageBlock)
			}
			send(`{"choices":[{"delta":{"content":"看到了"}}]}`)
		default:
			t.Fatalf("unexpected request %d", requestCount)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer modelServer.Close()

	client := NewChatClient()
	cfg := model.ModelConfig{BaseURL: modelServer.URL, Model: "fake", HideThinking: true}
	tools := []mcp.MCPTool{{Name: "image", FullName: "image_loader", Description: "load image", InputSchema: map[string]any{"type": "object"}}}
	var resultEvents []map[string]any
	answer, err := client.CompleteWithMCPToolsEvents(context.Background(), cfg, []model.Message{{Role: "user", Content: "看图"}}, tools, func(name string, args map[string]any) (any, error) {
		return map[string]any{
			"url":            "https://example.com/cat.png",
			"model_delivery": "image_url",
			toolModelContentKey: []map[string]any{
				{"type": "text", "text": "请分析这张图"},
				{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/cat.png"}},
			},
		}, nil
	}, func(kind string, payload any) error {
		if kind == "tool_call_result" {
			resultEvents = append(resultEvents, payload.(map[string]any))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "看到了" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if requestCount != 2 {
		t.Fatalf("expected model to receive tool image on second request, got %d", requestCount)
	}
	if len(resultEvents) != 1 || strings.Contains(fmt.Sprint(resultEvents[0]), "_chatdock_model_content") {
		t.Fatalf("tool result event should be sanitized, got %#v", resultEvents)
	}
}

func TestCompleteWithMCPToolsEventsHasNoFixedRoundCap(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["stream"] != true {
			t.Fatalf("tool-aware request %d should stream, got %#v", requestCount, body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if requestCount <= 9 {
			_, _ = w.Write([]byte(fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_%d\",\"type\":\"function\",\"function\":{\"name\":\"loop_tool\",\"arguments\":\"{\\\"round\\\":%d}\"}}]}}]}\n\n", requestCount, requestCount)))
			flusher.Flush()
		} else if requestCount == 10 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"完成"}}]}` + "\n\n"))
			flusher.Flush()
		} else {
			t.Fatalf("unexpected request %d", requestCount)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer modelServer.Close()

	client := NewChatClient()
	cfg := model.ModelConfig{BaseURL: modelServer.URL, Model: "fake", HideThinking: true}
	tools := []mcp.MCPTool{{Name: "loop", FullName: "loop_tool", Description: "loop", InputSchema: map[string]any{"type": "object"}}}
	calls := 0
	answer, err := client.CompleteWithMCPToolsEvents(context.Background(), cfg, []model.Message{{Role: "user", Content: "hi"}}, tools, func(name string, args map[string]any) (any, error) {
		calls++
		return map[string]any{"ok": true, "name": name, "args": args}, nil
	}, func(kind string, payload any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if answer != "完成" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if calls != 9 {
		t.Fatalf("expected 9 tool calls beyond old cap, got %d", calls)
	}
	if requestCount != 10 {
		t.Fatalf("expected final streamed answer on request 10, got %d", requestCount)
	}
}

func TestAppendMCPToolUseHint(t *testing.T) {
	messages := []map[string]any{{"role": "system", "content": "base"}, {"role": "user", "content": "hi"}}
	out := appendMCPToolUseHint(messages, []mcp.MCPTool{{Name: "read", FullName: "agentdock__read"}})
	if len(out) != 3 {
		t.Fatalf("expected hint to be inserted, got %#v", out)
	}
	if out[1]["role"] != "system" || !strings.Contains(out[1]["content"].(string), "MCP") {
		t.Fatalf("expected MCP system hint after existing system prompt, got %#v", out)
	}
}

func TestBuildChatMessagesAnyAppendsUploadedImageAsUserMessage(t *testing.T) {
	messages := BuildChatMessagesAny(model.ModelConfig{}, []model.Message{{
		Role:    "user",
		Content: "这张图是什么",
		ModelAttachments: []model.AttachmentRecord{{
			Attachment: model.Attachment{ID: "att_1", Name: "pixel.png", MIMEType: "image/png"},
			ModelURL:   "https://chatdock.200399.xyz/api/model-images/att_1?expires=123&sig=abc",
		}},
	}})
	if len(messages) != 2 {
		t.Fatalf("expected text message plus image message, got %#v", messages)
	}
	if messages[0]["role"] != "user" || messages[0]["content"] != "这张图是什么" {
		t.Fatalf("expected original user text to stay plain, got %#v", messages[0])
	}
	if messages[1]["role"] != "user" {
		t.Fatalf("expected appended image user message, got %#v", messages[1])
	}
	blocks, ok := messages[1]["content"].([]map[string]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("expected text and image blocks, got %#v", messages[1]["content"])
	}
	if blocks[0]["type"] != "text" || !strings.Contains(blocks[0]["text"].(string), "这张图是什么") {
		t.Fatalf("expected image prompt text block, got %#v", blocks[0])
	}
	imageURL, _ := blocks[1]["image_url"].(map[string]any)
	if blocks[1]["type"] != "image_url" || imageURL["url"] != "https://chatdock.200399.xyz/api/model-images/att_1?expires=123&sig=abc" {
		t.Fatalf("expected public image_url block, got %#v", blocks[1])
	}
}
