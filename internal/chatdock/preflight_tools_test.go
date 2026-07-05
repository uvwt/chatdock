package chatdock

import (
	"testing"

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
