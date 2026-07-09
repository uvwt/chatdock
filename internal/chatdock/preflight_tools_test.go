package chatdock

import (
	"context"
	"testing"

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
)

func TestDecideConversationPreflightProjectExplanation(t *testing.T) {
	decision := decideConversationPreflight([]model.Message{{Role: "user", Content: "ChatDock 强制调用工具建议吗？"}})
	if decision.NeedsMemory || decision.NeedsTaskTemplate {
		t.Fatalf("expected project explanation to rely on capability context only, got %+v", decision)
	}
}

func TestDecideConversationPreflightProjectMentionOnly(t *testing.T) {
	decision := decideConversationPreflight([]model.Message{{Role: "user", Content: "ChatDock 这个设计合理吗？"}})
	if decision.NeedsMemory || decision.NeedsTaskTemplate {
		t.Fatalf("expected project mention/discussion not to force tools, got %+v", decision)
	}
}

func TestDecideConversationPreflightProjectAction(t *testing.T) {
	decision := decideConversationPreflight([]model.Message{{Role: "user", Content: "按这个逻辑改 ChatDock 代码"}})
	if !decision.NeedsMemory || !decision.NeedsTaskTemplate {
		t.Fatalf("expected project action to need memory and task template, got %+v", decision)
	}
}

func TestDecideConversationPreflightContinueOperation(t *testing.T) {
	decision := decideConversationPreflight([]model.Message{
		{Role: "user", Content: "修改 ChatDock 前端并截图验证"},
		{Role: "assistant", Content: "已开始处理。"},
		{Role: "user", Content: "继续"},
	})
	if !decision.NeedsMemory || !decision.NeedsTaskTemplate {
		t.Fatalf("expected continuation after project operation to need memory and task template, got %+v", decision)
	}
}

func TestDecideConversationPreflightContinuePlainDiscussion(t *testing.T) {
	decision := decideConversationPreflight([]model.Message{
		{Role: "user", Content: "这个原理是什么？"},
		{Role: "assistant", Content: "它的核心是先判断意图。"},
		{Role: "user", Content: "继续"},
	})
	if decision.NeedsMemory || decision.NeedsTaskTemplate {
		t.Fatalf("expected plain continuation not to need tools, got %+v", decision)
	}
}

func TestConversationPreflightUsesWorkflowTemplateManageMatch(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "DockMini", Name: "workflow_template_manage", FullName: "DockMini__workflow_template_manage"},
	})
	var calledName string
	var calledArgs map[string]any
	result := (&App{}).runConversationPreflight(context.Background(), []model.Message{{Role: "user", Content: "按这个逻辑改 ChatDock 代码"}}, catalog, func(name string, args map[string]any) (any, error) {
		calledName = name
		calledArgs = args
		return map[string]any{"ok": true}, nil
	}, nil)

	if result.TaskTemplateError != "" {
		t.Fatalf("unexpected task template error: %s", result.TaskTemplateError)
	}
	if calledName != "DockMini__workflow_template_manage" {
		t.Fatalf("expected workflow_template_manage, got %q", calledName)
	}
	if calledArgs["action"] != "match" || calledArgs["goal"] == "" || calledArgs["task_type"] != nil {
		t.Fatalf("unexpected preflight args: %#v", calledArgs)
	}
}
