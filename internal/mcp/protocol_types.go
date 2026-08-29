package mcp

// MCPServerInfo 是一次成功协议协商后的稳定快照。
type MCPServerInfo struct {
	Server          string         `json:"server"`
	ProtocolVersion string         `json:"protocol_version"`
	Instructions    string         `json:"instructions,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	Implementation  map[string]any `json:"implementation,omitempty"`
}

// MCPServerInstruction 只携带进入模型上下文所需的外部 Server guidance。
type MCPServerInstruction struct {
	Server       string `json:"server"`
	Instructions string `json:"instructions"`
}

// MCPToolResult 保留 MCP tools/call 的标准结果字段。
type MCPToolResult struct {
	Content           []any          `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`

	appResource *MCPAppResource
	appError    string
}

// MCPAppResource 是 ChatDock Host 已通过 resources/read 读取并验证过的 MCP App。
type MCPAppResource struct {
	Server      string         `json:"server"`
	ResourceURI string         `json:"resource_uri"`
	MIMEType    string         `json:"mime_type"`
	HTML        string         `json:"html"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

func (r MCPToolResult) AppResource() *MCPAppResource {
	if r.appResource == nil {
		return nil
	}
	copy := *r.appResource
	copy.Meta = cloneJSONMap(r.appResource.Meta)
	return &copy
}

func (r MCPToolResult) AppError() string {
	return r.appError
}
