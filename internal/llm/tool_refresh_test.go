package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
)

func TestCompleteWithMCPToolsEventsRefreshesToolsAfterSearch(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		names := openAIToolNames(body["tools"])
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount {
		case 1:
			if !names["chatdock_tools_search"] || names["calendar__events_create"] {
				t.Fatalf("first round should expose search only, got %#v", names)
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_search\",\"type\":\"function\",\"function\":{\"name\":\"chatdock_tools_search\",\"arguments\":\"{\\\"query\\\":\\\"创建日历\\\"}\"}}]}}]}\n\ndata: [DONE]\n\n"))
		case 2:
			if !names["calendar__events_create"] {
				t.Fatalf("second round should contain the matched real tool, got %#v", names)
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_create\",\"type\":\"function\",\"function\":{\"name\":\"calendar__events_create\",\"arguments\":\"{\\\"title\\\":\\\"周会\\\"}\"}}]}}]}\n\ndata: [DONE]\n\n"))
		case 3:
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"已创建\"}}]}\n\ndata: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected request %d", requestCount)
		}
	}))
	defer modelServer.Close()

	searchTool := mcp.MCPTool{Name: "tools_search", FullName: "chatdock_tools_search", InputSchema: map[string]any{"type": "object"}}
	actualTool := mcp.MCPTool{Name: "events_create", FullName: "calendar__events_create", InputSchema: map[string]any{"type": "object"}}
	visibleTools := []mcp.MCPTool{searchTool}
	actualCalled := false
	client := NewChatClient()
	answer, err := client.CompleteWithMCPToolsEvents(
		context.Background(),
		model.ModelConfig{BaseURL: modelServer.URL, Model: "fake", HideThinking: true},
		[]model.Message{{Role: "user", Content: "创建日历"}},
		visibleTools,
		func(name string, args map[string]any) (any, error) {
			switch name {
			case "chatdock_tools_search":
				visibleTools = append(visibleTools, actualTool)
			case "calendar__events_create":
				actualCalled = true
			}
			return map[string]any{"ok": true}, nil
		},
		func(string, any) error { return nil },
		MCPToolLoopOptions{RefreshTools: func() []mcp.MCPTool { return append([]mcp.MCPTool(nil), visibleTools...) }},
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "已创建" || !actualCalled || requestCount != 3 {
		t.Fatalf("unexpected result: answer=%q actual_called=%v requests=%d", answer, actualCalled, requestCount)
	}
}

func TestCompleteWithMCPToolsEventsRecoversEmptyTerminalResponseAfterTools(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount {
		case 1:
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_status\",\"type\":\"function\",\"function\":{\"name\":\"status\",\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n\n"))
		case 2:
			if len(openAIToolNames(body["tools"])) == 0 {
				t.Fatal("normal post-tool round should still expose tools")
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{}}]}\n\ndata: [DONE]\n\n"))
		case 3:
			if _, ok := body["tools"]; ok {
				t.Fatalf("final recovery request must not expose tools: %#v", body["tools"])
			}
			messages, _ := body["messages"].([]any)
			last, _ := messages[len(messages)-1].(map[string]any)
			if last["role"] != "user" || last["content"] != finalToolResponseInstruction {
				t.Fatalf("unexpected final response instruction: %#v", last)
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"工具调用失败：缺少登录配置。\"}}]}\n\ndata: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected request %d", requestCount)
		}
	}))
	defer modelServer.Close()

	client := NewChatClient()
	answer, err := client.CompleteWithMCPToolsEvents(
		context.Background(),
		model.ModelConfig{BaseURL: modelServer.URL, Model: "fake"},
		[]model.Message{{Role: "user", Content: "检查状态"}},
		[]mcp.MCPTool{{Name: "status", FullName: "status", InputSchema: map[string]any{"type": "object"}}},
		func(string, map[string]any) (any, error) { return map[string]any{"ok": false}, nil },
		func(string, any) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "工具调用失败：缺少登录配置。" || requestCount != 3 {
		t.Fatalf("unexpected result: answer=%q requests=%d", answer, requestCount)
	}
}

func TestCompleteWithMCPToolsEventsRejectsRepeatedEmptyTerminalResponse(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/event-stream")
		if requestCount == 1 {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_status\",\"type\":\"function\",\"function\":{\"name\":\"status\",\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer modelServer.Close()

	client := NewChatClient()
	_, err := client.CompleteWithMCPToolsEvents(
		context.Background(),
		model.ModelConfig{BaseURL: modelServer.URL, Model: "fake"},
		[]model.Message{{Role: "user", Content: "检查状态"}},
		[]mcp.MCPTool{{Name: "status", FullName: "status", InputSchema: map[string]any{"type": "object"}}},
		func(string, map[string]any) (any, error) { return map[string]any{"ok": true}, nil },
		func(string, any) error { return nil },
	)
	if !errors.Is(err, ErrEmptyModelContent) || requestCount != 3 {
		t.Fatalf("expected ErrEmptyModelContent after one recovery, got err=%v requests=%d", err, requestCount)
	}
}

func TestCompleteWithMCPToolsBlockingRecoversEmptyTerminalResponseAfterTools(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_status","type":"function","function":{"name":"status","arguments":"{}"}}]}}]}`))
		case 2:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
		case 3:
			if _, ok := body["tools"]; ok {
				t.Fatalf("blocking final recovery request must not expose tools: %#v", body["tools"])
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"状态正常"}}]}`))
		default:
			t.Fatalf("unexpected request %d", requestCount)
		}
	}))
	defer modelServer.Close()

	client := NewChatClient()
	answer, err := client.CompleteWithMCPTools(
		context.Background(),
		model.ModelConfig{BaseURL: modelServer.URL, Model: "fake"},
		[]model.Message{{Role: "user", Content: "检查状态"}},
		[]mcp.MCPTool{{Name: "status", FullName: "status", InputSchema: map[string]any{"type": "object"}}},
		func(string, map[string]any) (any, error) { return map[string]any{"ok": true}, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "状态正常" || requestCount != 3 {
		t.Fatalf("unexpected result: answer=%q requests=%d", answer, requestCount)
	}
}

func TestCompleteWithMCPToolsBlockingMarksToolActivityBeforeSideEffect(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_status","type":"function","function":{"name":"status","arguments":"{}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"完成"}}]}`))
	}))
	defer modelServer.Close()

	marked := false
	client := NewChatClient()
	answer, err := client.CompleteWithMCPToolsEvents(
		context.Background(),
		model.ModelConfig{BaseURL: modelServer.URL, Model: "fake"},
		[]model.Message{{Role: "user", Content: "检查状态"}},
		[]mcp.MCPTool{{Name: "status", FullName: "status", InputSchema: map[string]any{"type": "object"}}},
		func(string, map[string]any) (any, error) {
			if !marked {
				t.Fatal("tool activity must be marked before the tool side effect runs")
			}
			return map[string]any{"ok": true}, nil
		},
		nil,
		MCPToolLoopOptions{OnToolCall: func() { marked = true }},
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "完成" || !marked || requestCount != 2 {
		t.Fatalf("unexpected blocking tool result: answer=%q marked=%v requests=%d", answer, marked, requestCount)
	}
}

func openAIToolNames(value any) map[string]bool {
	names := map[string]bool{}
	tools, _ := value.([]any)
	for _, item := range tools {
		tool, _ := item.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name != "" {
			names[name] = true
		}
	}
	return names
}
