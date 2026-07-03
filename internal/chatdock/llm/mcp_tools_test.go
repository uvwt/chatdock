package llm

import "chatdock/internal/chatdock/mcp"

import "testing"

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
