package httpapi

import (
	"context"
	"strings"

	"chatdock/internal/mcp"
)

type builtinToolHandler func(*Server, context.Context, map[string]any) (any, error)

type builtinToolRegistration struct {
	Tool    mcp.MCPTool
	Handler builtinToolHandler
}

func builtinChatDockToolRegistrations() []builtinToolRegistration {
	registrations := builtinScheduledTaskToolRegistrations()
	registrations = append(registrations, builtinImageToolRegistrations()...)
	registrations = append(registrations, builtinModelProviderToolRegistrations()...)
	return registrations
}

func builtinToolsFromRegistrations(registrations []builtinToolRegistration) []mcp.MCPTool {
	tools := make([]mcp.MCPTool, 0, len(registrations))
	for _, registration := range registrations {
		tools = append(tools, registration.Tool)
	}
	return tools
}

func builtinChatDockTools() []mcp.MCPTool {
	return builtinToolsFromRegistrations(builtinChatDockToolRegistrations())
}

func builtinChatDockToolRegistration(name string) (builtinToolRegistration, bool) {
	name = strings.TrimSpace(name)
	for _, registration := range builtinChatDockToolRegistrations() {
		if registration.Tool.FullName == name {
			return registration, true
		}
	}
	return builtinToolRegistration{}, false
}
