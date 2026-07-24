package httpapi

import (
	"context"
	"strings"
	"testing"

	"chatdock/internal/mcp"
)

func TestSearchToolCatalogReturnsOnlyMatchingTools(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "calendar", Name: "events_list", FullName: "calendar__events_list", Title: "查询日历事件", Description: "读取指定日期的日历安排"},
		{Server: "files", Name: "document_read", FullName: "files__document_read", Title: "读取文档", Description: "读取文本文件内容"},
	})

	result, matches := searchToolCatalogWithMatches(context.Background(), nil, catalog, map[string]any{"query": "日历", "limit": 1})
	items, ok := result["tools"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one search result, got %#v", result["tools"])
	}
	if items[0]["name"] != "calendar__events_list" || len(matches) != 1 || matches[0].FullName != "calendar__events_list" {
		t.Fatalf("expected calendar tool to match, got items=%#v matches=%#v", items, matches)
	}
	if !strings.Contains(result["next"].(string), "直接调用") {
		t.Fatalf("expected direct-call guidance, got %#v", result["next"])
	}
}

func TestSearchToolCatalogDoesNotInjectUnmatchedTools(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "shell", Name: "command_run", FullName: "shell__command_run", Title: "执行命令"},
		{Server: "tasks", Name: "task_create", FullName: "tasks__task_create", Title: "创建任务"},
	})

	result, matches := searchToolCatalogWithMatches(context.Background(), nil, catalog, map[string]any{"query": "天气", "limit": 8})
	items, ok := result["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected tools payload: %#v", result["tools"])
	}
	if len(items) != 0 || len(matches) != 0 {
		t.Fatalf("unmatched tools must not be injected, got items=%#v matches=%#v", items, matches)
	}
}

func TestToolSearchTextUsesDeclaredMetadataAndSchema(t *testing.T) {
	tool := mcp.MCPTool{
		Server:      "weather",
		Name:        "forecast_get",
		FullName:    "weather__forecast_get",
		Title:       "天气预报",
		Description: "查询未来天气",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
	}

	text := toolSearchText(tool)
	for _, expected := range []string{"weather__forecast_get", "天气预报", "查询未来天气", "city"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("tool search text should contain %q: %q", expected, text)
		}
	}
}
