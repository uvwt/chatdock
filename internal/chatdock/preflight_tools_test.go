package chatdock

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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
	result := (&App{}).runConversationPreflight(context.Background(), []model.Message{{Role: "user", Content: "按这个逻辑改 ChatDock 代码"}}, catalog, func(_ context.Context, name string, args map[string]any) (any, error) {
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

func TestConversationPreflightRunsMemoryAndTemplateWithoutSharedStateRaces(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "DockMini", Name: "recall_bootstrap", FullName: "DockMini__recall_bootstrap"},
		{Server: "DockMini", Name: "workflow_template_manage", FullName: "DockMini__workflow_template_manage"},
	})
	var mu sync.Mutex
	calls := map[string]map[string]any{}
	result := (&App{}).runConversationPreflight(context.Background(), []model.Message{{Role: "user", Content: "按这个逻辑改 ChatDock 代码"}}, catalog, func(_ context.Context, name string, args map[string]any) (any, error) {
		mu.Lock()
		calls[name] = args
		mu.Unlock()
		return map[string]any{"tool": name}, nil
	}, nil)

	if result.MemoryError != "" || result.TaskTemplateError != "" {
		t.Fatalf("unexpected preflight errors: %+v", result)
	}
	if result.MemoryTool != "DockMini__recall_bootstrap" || result.TaskTemplateTool != "DockMini__workflow_template_manage" {
		t.Fatalf("unexpected selected tools: %+v", result)
	}
	if len(calls) != 2 {
		t.Fatalf("expected both preflight calls, got %#v", calls)
	}
}

func TestConversationPreflightReturnsEmitterFailureInMatchingBranch(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "DockMini", Name: "recall_bootstrap", FullName: "DockMini__recall_bootstrap"},
		{Server: "DockMini", Name: "workflow_template_manage", FullName: "DockMini__workflow_template_manage"},
	})
	emitErr := errors.New("persist preflight event")
	result := (&App{}).runConversationPreflight(context.Background(), []model.Message{{Role: "user", Content: "按这个逻辑改 ChatDock 代码"}}, catalog, func(_ context.Context, name string, args map[string]any) (any, error) {
		return map[string]any{"tool": name}, nil
	}, func(event string, value any) error {
		return emitErr
	})
	if result.MemoryError != emitErr.Error() || result.TaskTemplateError != emitErr.Error() {
		t.Fatalf("expected emitter error in both branches: %+v", result)
	}
}

func TestConversationPreflightTimeoutCancelsContextAwareTools(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{
		{Server: "DockMini", Name: "recall_bootstrap", FullName: "DockMini__recall_bootstrap"},
		{Server: "DockMini", Name: "workflow_template_manage", FullName: "DockMini__workflow_template_manage"},
	})
	canceled := make(chan struct{}, 2)
	result := (&App{}).runConversationPreflightWithin(context.Background(), []model.Message{{Role: "user", Content: "提交并部署 ChatDock"}}, catalog, func(ctx context.Context, name string, args map[string]any) (any, error) {
		<-ctx.Done()
		canceled <- struct{}{}
		return nil, ctx.Err()
	}, nil, 20*time.Millisecond)
	if !strings.Contains(result.MemoryError, context.DeadlineExceeded.Error()) || !strings.Contains(result.TaskTemplateError, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected timeout errors in both branches: %+v", result)
	}
	for range 2 {
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("preflight tool did not receive canceled context")
		}
	}
}

func TestConversationPreflightTimeoutDoesNotWaitForMisbehavingTool(t *testing.T) {
	catalog := newToolCatalog([]mcp.MCPTool{{Server: "DockMini", Name: "recall_bootstrap", FullName: "DockMini__recall_bootstrap"}})
	release := make(chan struct{})
	finished := make(chan struct{})
	events := make(chan string, 4)
	startedAt := time.Now()
	result := (&App{}).runConversationPreflightWithin(context.Background(), []model.Message{{Role: "user", Content: "按这个逻辑改 ChatDock 代码"}}, catalog, func(context.Context, string, map[string]any) (any, error) {
		<-release
		close(finished)
		return map[string]any{"late": true}, nil
	}, func(event string, value any) error {
		events <- event
		return nil
	}, 20*time.Millisecond)
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("preflight waited for a non-cooperative tool: %s", elapsed)
	}
	if !strings.Contains(result.MemoryError, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected timeout result, got %+v", result)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("misbehaving tool did not finish after release")
	}
	if len(events) != 1 {
		t.Fatalf("late preflight result event was emitted: %d events", len(events))
	}
	if event := <-events; event != "tool_call_start" {
		t.Fatalf("unexpected preflight event after timeout: %s", event)
	}
}
