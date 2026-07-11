package llm

import (
	"errors"
	"testing"

	"chatdock/internal/chatdock/mcp"
)

func TestMCPToolsToOpenAIToolsDoesNotMutateSchema(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"q": map[string]any{"type": "string"}}}
	tools := MCPToolsToOpenAITools([]mcp.MCPTool{{Server: "a", Name: "search", InputSchema: schema}})
	if len(tools) != 1 {
		t.Fatalf("expected one tool")
	}
	if _, ok := schema["type"]; ok {
		t.Fatal("normalizeJSONSchema should not mutate original schema")
	}
}

func TestExecuteModelToolCallsBuildsProtocolMessages(t *testing.T) {
	calls := []ModelToolCall{{Function: ModelToolCallFunc{Name: "demo_tool", Arguments: `{"value":1}`}}}
	var events []string
	toolMessages, modelMessages, err := executeModelToolCalls(calls, func(name string, args map[string]any) (any, error) {
		if name != "demo_tool" || args["value"] != float64(1) {
			t.Fatalf("unexpected tool call: name=%q args=%#v", name, args)
		}
		return map[string]any{"answer": "ok"}, nil
	}, func(event string, value any) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(toolMessages) != 1 || toolMessages[0]["role"] != "tool" || toolMessages[0]["tool_call_id"] != "call_0" {
		t.Fatalf("unexpected tool messages: %#v", toolMessages)
	}
	if len(modelMessages) != 0 {
		t.Fatalf("unexpected model messages: %#v", modelMessages)
	}
	if len(events) != 2 || events[0] != "tool_call_start" || events[1] != "tool_call_result" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestExecuteModelToolCallsReturnsEmitterError(t *testing.T) {
	emitErr := errors.New("persist tool event")
	called := false
	_, _, err := executeModelToolCalls(
		[]ModelToolCall{{Function: ModelToolCallFunc{Name: "demo_tool", Arguments: `{}`}}},
		func(string, map[string]any) (any, error) {
			called = true
			return nil, nil
		},
		func(string, any) error { return emitErr },
	)
	if !errors.Is(err, emitErr) {
		t.Fatalf("expected emitter error, got %v", err)
	}
	if called {
		t.Fatal("tool must not run when the start event cannot be persisted")
	}
}
