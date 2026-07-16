package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
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
