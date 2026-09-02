package llm

import (
	"fmt"
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

func TestFitRawMessagesForContextCompressesCompletedOversizedToolResult(t *testing.T) {
	cfg := model.ModelConfig{ContextWindowTokens: 8 * 1024, OutputReserveTokens: 1024}
	messages := []map[string]any{
		{"role": "user", "content": "开始"},
		{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call-1", "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}}}},
		{"role": "tool", "tool_call_id": "call-1", "content": strings.Repeat("tool result ", 6000)},
		{"role": "user", "content": "现在总结"},
	}
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "lookup", "description": "lookup", "parameters": map[string]any{"type": "object"}}}}
	fitted, budget, err := FitRawMessagesForContext(cfg, messages, tools)
	if err != nil {
		t.Fatal(err)
	}
	if len(fitted) != 2 || fitted[0]["role"] != "system" || fitted[1]["role"] != "user" {
		t.Fatalf("completed tool turn was not summarized as older history: %#v", fitted)
	}
	if fitted[1]["content"] != "现在总结" {
		t.Fatalf("current user message changed: %#v", fitted[1])
	}
	if !strings.Contains(fmt.Sprint(fitted[0]["content"]), "# 早期会话摘要") {
		t.Fatalf("completed tool turn summary missing: %#v", fitted[0])
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

func TestRawRecentStartProtectsLastUserOnly(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "user 1"},
		{"role": "assistant", "content": "assistant 1"},
		{"role": "user", "content": "user 2"},
		{"role": "assistant", "content": "assistant 2"},
	}
	if got := rawRecentStart(messages); got != 2 {
		t.Fatalf("rawRecentStart = %d, want 2", got)
	}

	messagesEndingWithUser := []map[string]any{
		{"role": "user", "content": "user 1"},
		{"role": "assistant", "content": "assistant 1"},
		{"role": "user", "content": "user 2"},
	}
	if got := rawRecentStart(messagesEndingWithUser); got != 2 {
		t.Fatalf("rawRecentStart = %d, want 2", got)
	}

	activeTurnMessages := []map[string]any{
		{"role": "user", "content": "user 1"},
		{"role": "assistant", "content": "assistant 1"},
		{"role": "user", "content": "user 2"},
		{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call_1"}}},
		{"role": "tool", "tool_call_id": "call_1", "content": "result 1"},
	}
	if got := rawRecentStart(activeTurnMessages); got != 2 {
		t.Fatalf("rawRecentStart = %d, want 2", got)
	}

	noUserMessages := []map[string]any{
		{"role": "system", "content": "sys"},
		{"role": "assistant", "content": "assistant 1"},
	}
	if got := rawRecentStart(noUserMessages); got != len(noUserMessages) {
		t.Fatalf("rawRecentStart = %d, want %d", got, len(noUserMessages))
	}
}

func TestFitRawMessagesForContextActiveTurnPreservesToolCallPairingAndArguments(t *testing.T) {
	cfg := model.ModelConfig{ContextWindowTokens: 8 * 1024, OutputReserveTokens: 1024}
	largeLog := strings.Repeat("log-entry-line-data\n", 2000)
	rawArgs1 := `{"log_id":"app-123","query":"ERROR","limit":1000}`
	rawArgs2 := `{"service":"auth","details":true}`

	messages := []map[string]any{
		{"role": "user", "content": "请分析日志并检查服务"},
		{
			"role":    "assistant",
			"content": "",
			"tool_calls": []any{
				map[string]any{
					"id":   "call_log_1",
					"type": "function",
					"function": map[string]any{
						"name":      "fetch_log",
						"arguments": rawArgs1,
					},
				},
			},
		},
		{
			"role":         "tool",
			"tool_call_id": "call_log_1",
			"name":         "fetch_log",
			"content":      largeLog,
		},
		{
			"role":    "assistant",
			"content": "",
			"tool_calls": []any{
				map[string]any{
					"id":   "call_status_2",
					"type": "function",
					"function": map[string]any{
						"name":      "check_status",
						"arguments": rawArgs2,
					},
				},
			},
		},
		{
			"role":         "tool",
			"tool_call_id": "call_status_2",
			"name":         "check_status",
			"content":      `{"status":"ok","latency_ms":12}`,
		},
	}

	tools := []map[string]any{
		{"type": "function", "function": map[string]any{"name": "fetch_log", "description": "fetch logs", "parameters": map[string]any{"type": "object"}}},
		{"type": "function", "function": map[string]any{"name": "check_status", "description": "check service status", "parameters": map[string]any{"type": "object"}}},
	}

	fitted, budget, err := FitRawMessagesForContext(cfg, messages, tools)
	if err != nil {
		t.Fatalf("FitRawMessagesForContext failed: %v", err)
	}

	if len(fitted) != len(messages) {
		t.Fatalf("message length mismatch: got %d, want %d", len(fitted), len(messages))
	}

	// 1) 验证第一个超大 tool 结果已被折叠为 stub，且包含折叠标记
	tool1Content, _ := fitted[2]["content"].(string)
	if !strings.Contains(tool1Content, "tool_context_budget") || len(tool1Content) >= len(largeLog) {
		t.Fatalf("oldest tool result was not stubbed: %q", tool1Content)
	}

	// 2) 验证所有 assistant tool_calls 的 arguments 与 id 保持不变
	asst1Calls := fitted[1]["tool_calls"].([]any)
	asst1Call0 := asst1Calls[0].(map[string]any)
	if asst1Call0["id"] != "call_log_1" {
		t.Fatalf("assistant 1 call id changed: %v", asst1Call0["id"])
	}
	asst1Fn := asst1Call0["function"].(map[string]any)
	if asst1Fn["name"] != "fetch_log" || asst1Fn["arguments"] != rawArgs1 {
		t.Fatalf("assistant 1 arguments changed: %#v", asst1Fn)
	}

	asst2Calls := fitted[3]["tool_calls"].([]any)
	asst2Call0 := asst2Calls[0].(map[string]any)
	if asst2Call0["id"] != "call_status_2" {
		t.Fatalf("assistant 2 call id changed: %v", asst2Call0["id"])
	}
	asst2Fn := asst2Call0["function"].(map[string]any)
	if asst2Fn["name"] != "check_status" || asst2Fn["arguments"] != rawArgs2 {
		t.Fatalf("assistant 2 arguments changed: %#v", asst2Fn)
	}

	// 3) 验证 tool.tool_call_id 与 assistant.tool_calls.id 完整配对
	if fitted[2]["tool_call_id"] != "call_log_1" {
		t.Fatalf("tool 1 tool_call_id mismatch: %v", fitted[2]["tool_call_id"])
	}
	if fitted[4]["tool_call_id"] != "call_status_2" {
		t.Fatalf("tool 2 tool_call_id mismatch: %v", fitted[4]["tool_call_id"])
	}

	if budget.TotalTokens > budget.AvailableInputTokens {
		t.Fatalf("fitted budget exceeds available input tokens: %#v", budget)
	}
}

func TestProductionFailureShape128EventsFitsWithinBudget(t *testing.T) {
	// 生产故障配置：32K window, 4K reserve, available input budget = 25396
	cfg := model.ModelConfig{
		ContextWindowTokens: 32 * 1024,
		OutputReserveTokens: 4 * 1024,
	}

	// 模拟上一轮 assistant 执行了 128 个工具调用（展开为 128 个事件）
	events := make([]model.MessageEvent, 128)
	for i := 0; i < 128; i++ {
		toolName := "NexusDock__exec_command"
		events[i] = model.MessageEvent{
			ID:      fmt.Sprintf("event_%d", i),
			Kind:    "tool",
			Phase:   "done",
			CallKey: fmt.Sprintf("chatdock_tool_call::{\"tool\":%q}", toolName),
			Text:    "调用完成：" + toolName,
			Details: map[string]any{
				"event": "tool_call_result",
				"tool":  "chatdock_tool_call",
				"arguments": map[string]any{
					"tool": toolName,
					"arguments": map[string]any{
						"cmd":     fmt.Sprintf("git status --porcelain && ls -la dir_%d", i),
						"workdir": "/workspace/repo",
					},
				},
				"data": map[string]any{
					"ok":   true,
					"tool": toolName,
					"result": map[string]any{
						"stdout": fmt.Sprintf("M file_%d.go\n?? untracked_%d.go\n", i, i),
					},
				},
			},
		}
	}

	history := []model.Message{
		{ID: "user-1", Role: "user", Content: "请排查工作区所有文件状态并执行 128 项检测"},
		{
			ID:      "assistant-1",
			Role:    "assistant",
			Content: "已完成 128 项排查，工作区状态已核对完毕。",
			Events:  events,
		},
		{ID: "user-2", Role: "user", Content: "根据刚才的排查结果，做最终汇总"},
	}

	tools := []map[string]any{
		{"type": "function", "function": map[string]any{"name": "chatdock_tools_search", "description": "search tools", "parameters": map[string]any{"type": "object"}}},
		{"type": "function", "function": map[string]any{"name": "chatdock_tool_call", "description": "call tool", "parameters": map[string]any{"type": "object"}}},
	}

	// 1) 验证 BuildChatMessagesAnyChecked
	messages, _, err := BuildChatMessagesAnyChecked(cfg, history)
	if err != nil {
		t.Fatalf("BuildChatMessagesAnyChecked failed: %v", err)
	}

	// 验证 Completed Turn 不输出 role:tool 或 tool_calls
	toolRoleMsgs := messagesWithRole(messages, "tool")
	if len(toolRoleMsgs) != 0 {
		t.Fatalf("expected 0 tool role messages in completed history, got %d", len(toolRoleMsgs))
	}
	for _, msg := range messages {
		if _, hasCalls := msg["tool_calls"]; hasCalls {
			t.Fatalf("expected no tool_calls in completed history: %#v", msg)
		}
	}

	// 2) 验证 FitRawMessagesForContext
	fitted, fitBudget, fitErr := FitRawMessagesForContext(cfg, messages, tools)
	if fitErr != nil {
		t.Fatalf("FitRawMessagesForContext returned error on 128-event history: %v", fitErr)
	}

	if fitBudget.TotalTokens > fitBudget.AvailableInputTokens {
		t.Fatalf("total tokens %d exceeds available input tokens %d (budget: %#v)",
			fitBudget.TotalTokens, fitBudget.AvailableInputTokens, fitBudget)
	}

	if len(fitted) == 0 {
		t.Fatal("fitted messages should not be empty")
	}
}
