package llm

import (
	"errors"
	"strings"
	"testing"

	"chatdock/internal/mcp"
)

func TestMCPToolsToOpenAIToolsDoesNotMutateSchema(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"q": map[string]any{"type": "string"}}}
	tools := MCPToolsToOpenAITools([]mcp.MCPTool{{Server: "a", Name: "search", InputSchema: schema}})
	if len(tools) != 1 {
		t.Fatalf("expected one tool")
	}
	if _, ok := schema["type"]; ok {
		t.Fatal("model schema adaptation should not mutate original schema")
	}
}

func TestMCPToolsToOpenAIToolsFlattensTopLevelCompositionWithoutMutatingOriginal(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"artifact_id": map[string]any{"type": "string"},
			"path":        map[string]any{"type": "string"},
		},
		"oneOf": []any{
			map[string]any{"type": "object", "required": []any{"artifact_id"}},
			map[string]any{"type": "object", "required": []any{"path"}},
		},
	}

	tools := MCPToolsToOpenAITools([]mcp.MCPTool{{Server: "agentdock", Name: "view_image", InputSchema: schema}})
	function := tools[0]["function"].(map[string]any)
	parameters := function["parameters"].(map[string]any)
	if _, ok := parameters["oneOf"]; ok {
		t.Fatalf("top-level oneOf must be removed for model API compatibility: %#v", parameters)
	}
	properties := parameters["properties"].(map[string]any)
	for _, field := range []string{"artifact_id", "path"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("flattened schema does not declare field %q: %#v", field, parameters)
		}
	}
	description, _ := parameters["description"].(string)
	if !strings.Contains(description, "exactly one") || !strings.Contains(description, "artifact_id") || !strings.Contains(description, "path") {
		t.Fatalf("flattened schema lost composition guidance: %q", description)
	}
	if _, ok := schema["oneOf"].([]any)[0].(map[string]any)["properties"]; ok {
		t.Fatalf("input schema was mutated: %#v", schema)
	}
}

func TestAdaptToolSchemaForModelAPIFlattensAllTopLevelCompositionKeywords(t *testing.T) {
	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		t.Run(keyword, func(t *testing.T) {
			schema := map[string]any{
				"type": "object",
				keyword: []any{
					map[string]any{
						"type":       "object",
						"properties": map[string]any{"value": map[string]any{"type": "string"}},
						"required":   []any{"value"},
					},
				},
			}
			adapted := adaptToolSchemaForModelAPI(schema)
			if _, ok := adapted[keyword]; ok {
				t.Fatalf("top-level %s was not removed: %#v", keyword, adapted)
			}
			if _, ok := adapted["properties"].(map[string]any)["value"]; !ok {
				t.Fatalf("branch properties were not promoted: %#v", adapted)
			}
			if got := toolSchemaStringList(adapted["required"]); len(got) != 1 || got[0] != "value" {
				t.Fatalf("required fields were not preserved: %#v", adapted)
			}
		})
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
