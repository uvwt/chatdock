package chatdock

import "chatdock/internal/chatdock/mcp"

func builtinChatDockTools() []mcp.MCPTool {
	tools := builtinScheduledTaskTools()
	tools = append(tools, builtinImageTools()...)
	tools = append(tools, builtinModelProviderTools()...)
	return tools
}

func isBuiltinChatDockTool(name string) bool {
	return isBuiltinScheduledTaskTool(name) || isBuiltinImageTool(name) || isBuiltinModelProviderTool(name)
}
