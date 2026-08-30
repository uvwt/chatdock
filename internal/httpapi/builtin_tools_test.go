package httpapi

import (
	"context"
	"testing"

	"chatdock/internal/mcp"
)

func callBuiltinToolForTest(app *Server, name string, args map[string]any) (any, error) {
	return app.callConversationTool(context.Background(), "", mcp.MCPConfig{}, name, args, nil)
}

func TestBuiltinToolRegistrationsAreCompleteAndUnique(t *testing.T) {
	registrations := builtinChatDockToolRegistrations()
	seen := make(map[string]bool, len(registrations))
	for _, registration := range registrations {
		name := registration.Tool.FullName
		if name == "" {
			t.Fatal("builtin registration has empty full name")
		}
		if registration.Handler == nil {
			t.Fatalf("builtin registration %s has no handler", name)
		}
		if seen[name] {
			t.Fatalf("duplicate builtin registration: %s", name)
		}
		seen[name] = true
	}

	tools := builtinChatDockTools()
	if len(tools) != len(registrations) {
		t.Fatalf("builtin tools = %d, registrations = %d", len(tools), len(registrations))
	}
	for _, tool := range tools {
		if !seen[tool.FullName] {
			t.Fatalf("builtin tool is not backed by a registration: %s", tool.FullName)
		}
	}
}
