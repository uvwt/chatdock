package httpapi

import "chatdock/internal/mcp"

func builtinChatDockTools() []mcp.MCPTool {
	tools := builtinScheduledTaskTools()
	tools = append(tools, builtinImageTools()...)
	tools = append(tools, builtinModelProviderTools()...)
	return tools
}

func isBuiltinChatDockTool(name string) bool {
	return isBuiltinScheduledTaskTool(name) || isBuiltinImageTool(name) || isBuiltinModelProviderTool(name)
}
