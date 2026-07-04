package chatdock

import (
	"fmt"
	"sort"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

const (
	builtinToolSearchTools       = "chatdock_tools_search"
	builtinToolDescribeTools     = "chatdock_tools_describe"
	builtinToolExecuteDiscovered = "chatdock_tool_execute"
	builtinToolServerDiscovery   = "chatdock"
)

func builtinToolDiscoveryTools() []mcp.MCPTool {
	return []mcp.MCPTool{
		{
			Server:      builtinToolServerDiscovery,
			Name:        "tools_search",
			FullName:    builtinToolSearchTools,
			Title:       "查找可用工具",
			Description: "按用户目标或关键词查找当前可用工具。先用它找候选工具，再用 chatdock_tools_describe 获取具体参数。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "用户目标、关键词或能力名称，例如：记忆 搜索、浏览器 截图、定时任务、VPS 部署"},
				"limit": map[string]any{"type": "integer", "description": "最多返回多少个候选工具，默认 8，最大 20"},
			}, "required": []string{"query"}},
		},
		{
			Server:      builtinToolServerDiscovery,
			Name:        "tools_describe",
			FullName:    builtinToolDescribeTools,
			Title:       "查看工具详情",
			Description: "获取一个或多个候选工具的完整参数 schema。调用真实工具前必须先查看对应工具详情。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"names": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "要查看详情的工具 full_name 列表"},
			}, "required": []string{"names"}},
		},
		{
			Server:      builtinToolServerDiscovery,
			Name:        "tool_execute",
			FullName:    builtinToolExecuteDiscovered,
			Title:       "执行已查看工具",
			Description: "执行已经通过 chatdock_tools_describe 查看过详情的工具。name 必须是工具 full_name，arguments 必须符合该工具 schema。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"name":      map[string]any{"type": "string", "description": "要执行的工具 full_name"},
				"arguments": map[string]any{"type": "object", "description": "传给目标工具的参数对象，必须符合工具详情里的 schema"},
			}, "required": []string{"name", "arguments"}},
		},
	}
}

func isBuiltinToolDiscoveryTool(name string) bool {
	switch name {
	case builtinToolSearchTools, builtinToolDescribeTools, builtinToolExecuteDiscovered:
		return true
	default:
		return false
	}
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

func (c toolCatalog) Search(args map[string]any) map[string]any {
	query := strings.TrimSpace(stringArg(args, "query"))
	limit := intArgWithDefault(args, "limit", 8, 1, 20)
	matches := c.search(query, limit)
	items := make([]map[string]any, 0, len(matches))
	for _, tool := range matches {
		items = append(items, map[string]any{
			"name":        tool.FullName,
			"server":      tool.Server,
			"title":       firstNonEmpty(tool.Title, tool.Name),
			"description": compactToolDescription(tool.Description),
		})
	}
	return map[string]any{"query": query, "count": len(items), "tools": items, "next": "调用 chatdock_tools_describe，传入候选工具的 name，获取参数 schema；然后用 chatdock_tool_execute 执行。"}
}

func (c toolCatalog) Describe(args map[string]any) (map[string]any, []string) {
	names := toolNamesFromArgs(args)
	items := make([]map[string]any, 0, len(names))
	missing := make([]string, 0)
	described := make([]string, 0, len(names))
	for _, name := range names {
		tool, ok := c.byName[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		described = append(described, name)
		items = append(items, map[string]any{
			"name":         tool.FullName,
			"server":       tool.Server,
			"title":        firstNonEmpty(tool.Title, tool.Name),
			"description":  tool.Description,
			"input_schema": tool.InputSchema,
			"execute_with": builtinToolExecuteDiscovered,
		})
	}
	return map[string]any{"tools": items, "missing": missing, "next": "调用 chatdock_tool_execute，name 使用工具 full_name，arguments 按 input_schema 填写。"}, described
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

func toolNamesFromArgs(args map[string]any) []string {
	value, ok := args["names"]
	if !ok || value == nil {
		if name := strings.TrimSpace(stringArg(args, "name")); name != "" {
			return []string{name}
		}
		return nil
	}
	var names []string
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if name := strings.TrimSpace(fmt.Sprint(item)); name != "" {
				names = append(names, name)
			}
		}
	case []string:
		names = append(names, v...)
	case string:
		for _, item := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == '\t' }) {
			if name := strings.TrimSpace(item); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

func mapArg(args map[string]any, key string) map[string]any {
	value, ok := args[key]
	if !ok || value == nil {
		return map[string]any{}
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{"_raw_arguments": value}
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
