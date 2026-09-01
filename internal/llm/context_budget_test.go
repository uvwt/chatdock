package llm

import (
	"strings"
	"testing"

	"chatdock/internal/model"
)

func TestContextBudgetUsesModelWindowAndReservesSafety(t *testing.T) {
	tests := []struct {
		name      string
		window    int
		reserve   int
		safety    int
		available int
		trigger   int
		target    int
	}{
		{name: "8K", window: 8 * 1024, reserve: 1024, safety: 2 * 1024, available: 5 * 1024, trigger: 1536, target: 768},
		{name: "32K", window: 32 * 1024, reserve: 4 * 1024, safety: 3276, available: 25396, trigger: 7618, target: 3809},
		{name: "128K", window: 128 * 1024, reserve: 8 * 1024, safety: 13107, available: 109773, trigger: 32931, target: 16465},
		{name: "custom", window: 50 * 1024, reserve: 5 * 1024, safety: 5120, available: 40960, trigger: 12288, target: 6144},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget := ContextBudgetForConfig(model.ModelConfig{ContextWindowTokens: tt.window, OutputReserveTokens: tt.reserve})
			if budget.SafetyMarginTokens != tt.safety || budget.AvailableInputTokens != tt.available || budget.CompressionTriggerTokens != tt.trigger || budget.CompressionTargetTokens != tt.target {
				t.Fatalf("budget = %#v", budget)
			}
		})
	}
}

func TestPrepareChatContextUsesTokensInsteadOfMessageCount(t *testing.T) {
	cfg := model.ModelConfig{SystemPrompt: "sys", ContextWindowTokens: 8 * 1024, OutputReserveTokens: 1024}
	large := []model.Message{
		{Role: "user", Content: strings.Repeat("large history ", 900)},
		{Role: "assistant", Content: "older answer"},
		{Role: "user", Content: "recent question"},
		{Role: "assistant", Content: "recent answer"},
		{Role: "user", Content: "current question"},
	}
	prepared, err := PrepareChatContext(cfg, large)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Compressed || !prepared.Budget.NextCompression || !strings.Contains(prepared.Summary, "large history") {
		t.Fatalf("large history was not compressed: %#v", prepared)
	}
	if got := prepared.Messages[len(prepared.Messages)-3].Content; got != "recent question" {
		t.Fatalf("latest complete turn was not kept: %#v", prepared.Messages)
	}
	if got := prepared.Messages[len(prepared.Messages)-1].Content; got != "current question" {
		t.Fatalf("current user message was not kept: %#v", prepared.Messages)
	}

	short := make([]model.Message, 0, 100)
	for i := 0; i < 100; i++ {
		short = append(short, model.Message{Role: "user", Content: "ok"})
	}
	shortPrepared, err := PrepareChatContext(cfg, short)
	if err != nil {
		t.Fatal(err)
	}
	if shortPrepared.Compressed || shortPrepared.Budget.NextCompression {
		t.Fatalf("many short messages were compressed by count: %#v", shortPrepared.Budget)
	}
}

func TestFitRawMessagesForContextFoldsOversizedToolResult(t *testing.T) {
	cfg := model.ModelConfig{ContextWindowTokens: 8 * 1024, OutputReserveTokens: 1024}
	originalToolResult := strings.Repeat("tool result ", 6000)
	messages := []map[string]any{
		{"role": "user", "content": "开始"},
		{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call-1", "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}}}},
		{"role": "tool", "tool_call_id": "call-1", "content": originalToolResult},
		{"role": "user", "content": "现在总结"},
	}
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "lookup", "description": "lookup", "parameters": map[string]any{"type": "object"}}}}
	fitted, budget, err := FitRawMessagesForContext(cfg, messages, tools)
	if err != nil {
		t.Fatal(err)
	}
	if len(fitted) != len(messages) {
		t.Fatalf("tool protocol messages were dropped: %#v", fitted)
	}
	toolContent, _ := fitted[2]["content"].(string)
	if len(toolContent) >= len(originalToolResult) || !strings.Contains(toolContent, "tool_context_budget") {
		t.Fatalf("oversized tool result was not folded: %q", toolContent)
	}
	if budget.TotalTokens > budget.AvailableInputTokens {
		t.Fatalf("fitted request remains over budget: %#v", budget)
	}
}

func TestPrepareChatContextRejectsOversizedFixedPrefix(t *testing.T) {
	_, err := PrepareChatContext(model.ModelConfig{SystemPrompt: strings.Repeat("固定提示词 ", 10000), ContextWindowTokens: 8 * 1024, OutputReserveTokens: 1024}, []model.Message{{Role: "user", Content: "hello"}})
	if err == nil || !strings.Contains(err.Error(), ErrContextBudgetExceeded.Error()) {
		t.Fatalf("oversized fixed prefix error = %v", err)
	}
}
