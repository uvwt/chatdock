package chatdock

import (
	"context"
	"strings"
	"testing"

	"chatdock/internal/chatdock/mcp"
)

func TestSearchToolCatalogAlwaysReturnsPinnedTools(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "DockMini", Name: "exec_command", FullName: "DockMini__exec_command", Title: "执行命令"},
		{Server: "DockMini", Name: "skill_read", FullName: "DockMini__skill_read", Title: "Skill 发现"},
		{Server: "DockMini", Name: "skill_run", FullName: "DockMini__skill_run", Title: "Skill 执行"},
		{Server: "DockMini", Name: "workflow_template_manage", FullName: "DockMini__workflow_template_manage", Title: "任务模板"},
		{Server: "DockMini", Name: "task_manage", FullName: "DockMini__task_manage", Title: "任务模板"},
	})

	result := searchToolCatalogWithApp(context.Background(), nil, "default", catalog, map[string]any{"query": "天气", "limit": 1})
	items, ok := result["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected tools payload: %#v", result["tools"])
	}
	assertToolSearchItem(t, items, "DockMini__exec_command")
	assertToolSearchItem(t, items, "DockMini__skill_read")
	assertToolSearchItem(t, items, "DockMini__skill_run")
	assertToolSearchItem(t, items, "DockMini__workflow_template_manage")
	assertToolSearchItem(t, items, "DockMini__task_manage")
}

func TestPinnedToolMatchesSupportSingleUnderscoreNames(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "DockMini", Name: "exec_command", FullName: "DockMini_exec_command", Title: "执行命令"},
		{Server: "DockMini", Name: "skill_read", FullName: "DockMini_skill_read", Title: "Skill 发现"},
		{Server: "DockMini", Name: "skill_run", FullName: "DockMini_skill_run", Title: "Skill 执行"},
		{Server: "DockMini", Name: "workflow_template_manage", FullName: "DockMini_workflow_template_manage", Title: "任务模板"},
		{Server: "DockMini", Name: "task_manage", FullName: "DockMini_task_manage", Title: "任务模板"},
	})

	matches := appendPinnedToolMatches(catalog, nil)

	assertPinnedTool(t, matches, "DockMini_exec_command")
	assertPinnedTool(t, matches, "DockMini_skill_read")
	assertPinnedTool(t, matches, "DockMini_skill_run")
	assertPinnedTool(t, matches, "DockMini_workflow_template_manage")
	assertPinnedTool(t, matches, "DockMini_task_manage")
}

func TestPinnedToolSearchAliasesBoostExecCommandTerms(t *testing.T) {
	tool := mcp.MCPTool{Name: "exec_command", FullName: "DockMini__exec_command"}
	text := pinnedToolSearchAliases(tool)
	if !strings.Contains(text, "执行命令") || !strings.Contains(text, "docker") || !strings.Contains(text, "go test") {
		t.Fatalf("exec_command aliases should include command keywords: %q", text)
	}
}

func assertToolSearchItem(t *testing.T, items []map[string]any, name string) {
	t.Helper()
	for _, item := range items {
		if item["name"] == name {
			return
		}
	}
	t.Fatalf("missing searched tool %s in %#v", name, items)
}

func assertPinnedTool(t *testing.T, matches []hybridToolMatch, name string) {
	t.Helper()
	for _, match := range matches {
		if match.tool.FullName == name {
			if !match.pinned {
				t.Fatalf("%s should be marked pinned", name)
			}
			return
		}
	}
	t.Fatalf("missing pinned tool %s in %#v", name, matches)
}
