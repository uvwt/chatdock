package model

type MCPConfigResponse struct {
	Content      string    `json:"content"`
	BuiltinTools []MCPTool `json:"builtin_tools"`
}

type SaveMCPConfigRequest struct {
	Content string `json:"content"`
}

type MCPToolsResponse struct {
	Tools []MCPTool `json:"tools"`
}

// MCPTool 是前端展示和模型工具转换共用的工具描述。
// 放在 model 包里，避免 httpapi、LLM 和 MCP 客户端互相反向依赖。
type MCPTool struct {
	Server      string         `json:"server"`
	Name        string         `json:"name"`
	FullName    string         `json:"full_name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}
