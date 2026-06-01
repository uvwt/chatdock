package chatdock

import "testing"

func TestParseMCPConfigAndToolFilters(t *testing.T) {
	content := `{
		"servers": {
			"agentdock": {
				"url": "http://127.0.0.1:18766/mcp",
				"allow_tools": ["memory_*"],
				"deny_tools": ["memory_delete"],
				"confirm_tools": ["memory_write"]
			}
		}
	}`
	cfg, err := ParseMCPConfig(content)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Servers["agentdock"]
	if !s.allowsTool("memory_read", "agentdock__memory_read") {
		t.Fatal("expected memory_read to be allowed")
	}
	if s.allowsTool("desktop_click", "agentdock__desktop_click") {
		t.Fatal("expected desktop_click to be filtered by allow list")
	}
	if s.allowsTool("memory_delete", "agentdock__memory_delete") {
		t.Fatal("expected memory_delete to be denied")
	}
	if !s.requiresConfirmation("memory_write", "agentdock__memory_write") {
		t.Fatal("expected memory_write to require confirmation")
	}
}

func TestMCPToolsToOpenAIToolsDoesNotMutateSchema(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"q": map[string]any{"type": "string"}}}
	tools := MCPToolsToOpenAITools([]MCPTool{{Server: "a", Name: "search", InputSchema: schema}})
	if len(tools) != 1 {
		t.Fatalf("expected one tool")
	}
	if _, ok := schema["type"]; ok {
		t.Fatal("normalizeJSONSchema should not mutate original schema")
	}
}
