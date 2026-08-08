package httpapi

import (
	"sort"
	"strings"

	"chatdock/internal/mcp"
)

const (
	builtinToolSearchTools     = "chatdock_tools_search"
	builtinToolServerDiscovery = "chatdock"
)

func builtinToolSearchTool(resources []toolResource) mcp.MCPTool {
	resourceIDs := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource.Status == "disabled" {
			continue
		}
		resourceIDs = append(resourceIDs, resource.ID)
	}
	description := "按用户目标查找并加载 ChatDock 内置工具或 MCP 工具。可以用 query 搜索；任务明确集中在一个资源时，可只传 resources 且不传 query，一次加载该资源全部真实工具。命中的真实工具会在下一次模型请求中直接出现，不通过代理工具执行。"
	if index := resourceIndexText(resources); index != "" {
		description += "\n\n当前资源索引：\n" + index
	}
	resourceItems := map[string]any{"type": "string", "description": "资源 ID"}
	if len(resourceIDs) > 0 {
		resourceItems["enum"] = resourceIDs
	}
	return mcp.MCPTool{
		Server:      builtinToolServerDiscovery,
		Name:        "tools_search",
		FullName:    builtinToolSearchTools,
		Title:       "查找或加载工具",
		Description: description,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":     map[string]any{"type": "string", "maxLength": maxToolDiscoveryQueryRunes, "description": "用户目标、关键词或能力名称；单资源全量加载时可省略"},
				"resources": map[string]any{"type": "array", "items": resourceItems, "maxItems": maxToolDiscoveryResourceCount, "uniqueItems": true, "description": "限定搜索或加载的资源；只传一个资源且省略 query 时加载该资源全部工具"},
				"limit":     map[string]any{"type": "integer", "description": "搜索时最多加载多少个候选工具，默认 8，最大 20"},
			},
			"additionalProperties": false,
		},
	}
}

func isBuiltinToolDiscoveryTool(name string) bool {
	return name == builtinToolSearchTools
}

type toolCatalog struct {
	tools  []mcp.MCPTool
	byName map[string]mcp.MCPTool
}

func newToolCatalog(tools []mcp.MCPTool) toolCatalog {
	catalog := toolCatalog{tools: make([]mcp.MCPTool, 0, len(tools)), byName: map[string]mcp.MCPTool{}}
	for _, tool := range tools {
		name := strings.TrimSpace(tool.FullName)
		if name == "" {
			name = mcp.ToolFullName(tool.Server, tool.Name)
			tool.FullName = name
		}
		if name == "" || isBuiltinToolDiscoveryTool(name) {
			continue
		}
		catalog.tools = append(catalog.tools, tool)
		catalog.byName[name] = tool
	}
	return catalog
}

func (c toolCatalog) Get(name string) (mcp.MCPTool, bool) {
	tool, ok := c.byName[strings.TrimSpace(name)]
	return tool, ok
}

func (c toolCatalog) search(query string, limit int) []mcp.MCPTool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		if len(c.tools) <= limit {
			return append([]mcp.MCPTool(nil), c.tools...)
		}
		return append([]mcp.MCPTool(nil), c.tools[:limit]...)
	}
	type scoredTool struct {
		tool  mcp.MCPTool
		score int
	}
	var scored []scoredTool
	terms := strings.Fields(query)
	if len(terms) == 0 {
		terms = []string{query}
	}
	for _, tool := range c.tools {
		text := strings.ToLower(strings.Join([]string{tool.FullName, tool.Server, tool.Name, tool.Title, tool.Description}, " "))
		score := 0
		for _, term := range terms {
			if strings.Contains(strings.ToLower(tool.FullName), term) || strings.Contains(strings.ToLower(tool.Name), term) {
				score += 5
			}
			if strings.Contains(strings.ToLower(tool.Title), term) {
				score += 3
			}
			if strings.Contains(text, term) {
				score++
			}
		}
		if score > 0 {
			scored = append(scored, scoredTool{tool: tool, score: score})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].tool.FullName < scored[j].tool.FullName
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]mcp.MCPTool, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.tool)
	}
	return out
}

func intArgWithDefault(args map[string]any, key string, fallback, minValue, maxValue int) int {
	value, ok, err := optionalIntArg(args, key)
	if err != nil || !ok {
		value = fallback
	}
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	return value
}

func compactToolDescription(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 180 {
		return string(runes[:180]) + "…"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
