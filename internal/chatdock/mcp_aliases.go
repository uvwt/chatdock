package chatdock

import "chatdock/internal/chatdock/mcp"

type MCPClient = mcp.MCPClient
type MCPConfig = mcp.MCPConfig
type MCPServerConfig = mcp.MCPServerConfig
type MCPAuthConfig = mcp.MCPAuthConfig
type MCPTool = mcp.MCPTool
type MCPToolCallRequest = mcp.MCPToolCallRequest
type MCPToolCallResponse = mcp.MCPToolCallResponse

func NewMCPClient() *MCPClient { return mcp.NewMCPClient() }

func ParseMCPConfig(content string) (MCPConfig, error) { return mcp.ParseMCPConfig(content) }

func splitToolFullName(fullName string) (string, string) { return mcp.SplitToolFullName(fullName) }

func compactJSON(value any) string { return mcp.CompactJSON(value) }
