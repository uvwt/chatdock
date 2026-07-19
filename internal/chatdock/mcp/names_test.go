package mcp

import (
	"reflect"
	"testing"
)

func TestToolFullNameSanitizesComponentsAndRoundTrips(t *testing.T) {
	fullName := ToolFullName(" calendar prod ", "events/list 中文")
	if fullName != "calendar_prod__events_list___" {
		t.Fatalf("full tool name = %q", fullName)
	}
	serverName, toolName := SplitToolFullName(fullName)
	if serverName != "calendar_prod" || toolName != "events_list___" {
		t.Fatalf("split tool name = %q, %q", serverName, toolName)
	}

	serverName, toolName = SplitToolFullName("read_file")
	if serverName != "agentdock" || toolName != "read_file" {
		t.Fatalf("legacy tool name fallback = %q, %q", serverName, toolName)
	}
	if got := ToolFullName("", ""); got != "tool__tool" {
		t.Fatalf("empty tool name = %q", got)
	}
}

func TestNormalizeJSONSchemaAddsDefaultsWithoutMutatingInput(t *testing.T) {
	input := map[string]any{"description": "demo"}
	normalized := NormalizeJSONSchema(input)
	if normalized["type"] != "object" {
		t.Fatalf("schema type = %#v", normalized["type"])
	}
	if properties, ok := normalized["properties"].(map[string]any); !ok || len(properties) != 0 {
		t.Fatalf("schema properties = %#v", normalized["properties"])
	}
	if !reflect.DeepEqual(input, map[string]any{"description": "demo"}) {
		t.Fatalf("input schema was mutated: %#v", input)
	}

	nilSchema := NormalizeJSONSchema(nil)
	if nilSchema["type"] != "object" {
		t.Fatalf("nil schema default = %#v", nilSchema)
	}
}

func TestCompactJSONUsesDeterministicJSONWhenSupported(t *testing.T) {
	if got := CompactJSON(map[string]any{"b": 2, "a": 1}); got != `{"a":1,"b":2}` {
		t.Fatalf("compact JSON = %q", got)
	}
}

func TestBearerTokenUsesInlineTokenBeforeEnvironment(t *testing.T) {
	t.Setenv("CHATDOCK_TEST_MCP_TOKEN", "Bearer environment-token")
	server := MCPServerConfig{Auth: MCPAuthConfig{
		Type:     " bearer ",
		Token:    "Bearer inline-token",
		TokenEnv: "CHATDOCK_TEST_MCP_TOKEN",
	}}
	if got := server.BearerToken(); got != "inline-token" {
		t.Fatalf("inline token precedence = %q", got)
	}
	server.Auth.Token = ""
	if got := server.BearerToken(); got != "environment-token" {
		t.Fatalf("environment token = %q", got)
	}
	server.Auth.Type = "none"
	if got := server.BearerToken(); got != "" {
		t.Fatalf("non-bearer token = %q", got)
	}
}
