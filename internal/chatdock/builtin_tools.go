package chatdock

import "chatdock/internal/chatdock/mcp"

func builtinChatDockTools() []mcp.MCPTool {
	tools := builtinScheduledTaskTools()
	tools = append(tools, builtinImageTools()...)
	return tools
}
