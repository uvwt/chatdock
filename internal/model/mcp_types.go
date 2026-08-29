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

// MCPTool 是 ChatDock 在协议边界归一化后的 MCP 工具定义。
// SDK 类型不向 HTTP、LLM 与前端扩散，避免上层代码跟随协议库版本变化。
type MCPTool struct {
	Server       string           `json:"server"`
	Name         string           `json:"name"`
	FullName     string           `json:"full_name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	InputSchema  map[string]any   `json:"input_schema,omitempty"`
	OutputSchema map[string]any   `json:"output_schema,omitempty"`
	Annotations  map[string]any   `json:"annotations,omitempty"`
	Meta         map[string]any   `json:"_meta,omitempty"`
	Icons        []map[string]any `json:"icons,omitempty"`
}
