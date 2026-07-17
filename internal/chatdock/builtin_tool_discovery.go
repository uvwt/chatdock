package chatdock

import (
	"sort"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

const (
	builtinToolSearchTools     = "chatdock_tools_search"
	builtinToolServerDiscovery = "chatdock"
)

func builtinToolSearchTool() mcp.MCPTool {
	return mcp.MCPTool{
		Server:      builtinToolServerDiscovery,
		Name:        "tools_search",
		FullName:    builtinToolSearchTools,
		Title:       "查找按需工具",
		Description: "按用户目标或关键词查找当前按需加载的 ChatDock 内置工具和 MCP 工具。命中的真实工具会在下一轮直接加入工具列表，无需查看详情或通过代理工具执行。",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "用户目标、关键词或能力名称"},
			"limit": map[string]any{"type": "integer", "description": "最多加载多少个候选工具，默认 8，最大 20"},
		}, "required": []string{"query"}},
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
