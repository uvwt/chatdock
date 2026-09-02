package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"chatdock/internal/model"
)

func TestBuildChatMessagesAnyCompletedTurnOutputsAssistantTraceOnly(t *testing.T) {
	history := []model.Message{
		{ID: "user-1", Role: "user", Content: "第一轮"},
		historicalToolTestMessage("assistant-1", "oldest_tool", "先查旧数据。", "旧数据完成。"),
		{ID: "user-2", Role: "user", Content: "第二轮"},
		historicalToolTestMessage("assistant-2", "middle_tool", "开始处理中间数据。", "中间数据完成。"),
		{ID: "user-3", Role: "user", Content: "第三轮"},
		historicalToolTestMessage("assistant-3", "newest_tool", "开始处理最新数据。", "最新数据完成。"),
		{ID: "user-4", Role: "user", Content: "继续"},
	}

	messages := BuildChatMessagesAny(model.ModelConfig{ContextMode: model.ContextModeCustom, MaxContextMessages: 20}, history)

	toolMessages := messagesWithRole(messages, "tool")
	if len(toolMessages) != 0 {
		t.Fatalf("expected 0 tool role messages, got %d: %#v", len(toolMessages), toolMessages)
	}
	for _, msg := range messages {
		if _, hasCalls := msg["tool_calls"]; hasCalls {
			t.Fatalf("completed turn should not output tool_calls: %#v", msg)
		}
	}

	assistantMessages := messagesWithRole(messages, "assistant")
	if len(assistantMessages) != 3 {
		t.Fatalf("expected 3 assistant messages, got %d: %#v", len(assistantMessages), assistantMessages)
	}

	// 历史超过 limit(2) 的最老助手消息只保留普通文本
	oldestContent, _ := assistantMessages[0]["content"].(string)
	if strings.Contains(oldestContent, "<tool_execution_trace") {
		t.Fatalf("oldest assistant message beyond limit should not contain trace: %s", oldestContent)
	}
	if !strings.Contains(oldestContent, "先查旧数据。") {
		t.Fatalf("oldest assistant text lost: %s", oldestContent)
	}

	// 最近两轮 completed assistant 包含纯文本 trace
	for _, assistant := range assistantMessages[1:] {
		content, _ := assistant["content"].(string)
		if !strings.Contains(content, "<tool_execution_trace") || !strings.Contains(content, "</tool_execution_trace>") {
			t.Fatalf("recent assistant message missing tool_execution_trace: %s", content)
		}
		if strings.Contains(content, "_chatdock_model_content") {
			t.Fatalf("internal model field leaked into trace: %s", content)
		}
	}

	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "不得回传的推理") {
		t.Fatalf("reasoning leaked into model history: %s", raw)
	}
}

func TestHistoricalToolMessageIndexesUsesAssistantTurns(t *testing.T) {
	history := []model.Message{
		historicalToolTestMessage("assistant-1", "first", "", "完成一"),
		{Role: "assistant", Content: "纯文本"},
		historicalToolTestMessage("assistant-2", "second", "", "完成二"),
		historicalToolTestMessage("assistant-3", "third", "", "完成三"),
	}
	indexes := HistoricalToolMessageIndexes(history)
	if len(indexes) != 2 || indexes[0] != 2 || indexes[1] != 3 {
		t.Fatalf("historical indexes = %#v, want [2 3]", indexes)
	}
}

func TestCompletedToolTraceDeterministicAndBounded(t *testing.T) {
	events := []model.MessageEvent{
		{
			Kind:  "tool",
			Phase: "done",
			Text:  "调用完成：read_file",
			Details: map[string]any{
				"event":     "tool_call_result",
				"tool":      "read_file",
				"arguments": map[string]any{"path": "/src/main.go", "offset": 100},
				"data": map[string]any{
					"ok":   true,
					"tool": "read_file",
					"result": map[string]any{
						"content": "package main\n\nfunc main() {}",
					},
				},
			},
		},
		{
			Kind:  "tool",
			Phase: "error",
			Text:  "调用失败：exec_cmd",
			Details: map[string]any{
				"event":     "tool_call_result",
				"tool":      "exec_cmd",
				"arguments": map[string]any{"cmd": "npm test", "workdir": "/app"},
				"error":     "exit status 1: test failed",
				"data": map[string]any{
					"ok":    false,
					"tool":  "exec_cmd",
					"error": "exit status 1: test failed",
				},
			},
		},
	}

	trace1 := completedToolTrace(events)
	trace2 := completedToolTrace(events)

	if trace1 != trace2 {
		t.Fatalf("trace is not byte-level deterministic:\ntrace1:\n%s\ntrace2:\n%s", trace1, trace2)
	}
	if tokens := EstimateTokens(trace1); tokens > completedToolTraceMaxTokens {
		t.Fatalf("trace tokens = %d, max = %d", tokens, completedToolTraceMaxTokens)
	}
	if !strings.Contains(trace1, "<tool_execution_trace total=2>") || !strings.Contains(trace1, "</tool_execution_trace>") {
		t.Fatalf("trace format invalid: %s", trace1)
	}
	if !strings.Contains(trace1, "[done] read_file") || !strings.Contains(trace1, "[error] exec_cmd") {
		t.Fatalf("trace missing status or tool name: %s", trace1)
	}
}

func TestCompletedToolTraceKeepsSmallStructuredOutcomeAndDropsLargeOutput(t *testing.T) {
	event := model.MessageEvent{
		Kind: "tool", Phase: "done", Text: "调用完成：deploy",
		Details: map[string]any{
			"event":     "tool_call_result",
			"tool":      "deploy",
			"arguments": map[string]any{"path": "/srv/app"},
			"data": map[string]any{
				"tool": "deploy",
				"result": map[string]any{
					"id":        "release-42",
					"status":    "healthy",
					"exit_code": 0,
					"stdout":    strings.Repeat("very long build output ", 200),
				},
			},
		},
	}
	trace := completedToolTrace([]model.MessageEvent{event})
	if !strings.Contains(trace, "release-42") || !strings.Contains(trace, "healthy") || !strings.Contains(trace, "exit_code") {
		t.Fatalf("small structured outcome was lost: %s", trace)
	}
	if strings.Contains(trace, "very long build output") {
		t.Fatalf("large stdout leaked into completed trace: %s", trace)
	}
}

func TestCompletedToolTrace128EventsBoundsAndPreservesHeadTailAndErrors(t *testing.T) {
	events := make([]model.MessageEvent, 128)
	for i := 0; i < 128; i++ {
		cmdName := fmt.Sprintf("echo step_%d", i)
		if i == 50 {
			events[i] = model.MessageEvent{
				ID:      fmt.Sprintf("event-%d", i),
				Kind:    "tool",
				Phase:   "error",
				CallKey: "chatdock_tool_call::{}",
				Text:    "调用失败：NexusDock__exec_command",
				Details: map[string]any{
					"event": "tool_call_result",
					"tool":  "chatdock_tool_call",
					"arguments": map[string]any{
						"tool": "NexusDock__exec_command",
						"arguments": map[string]any{
							"cmd":     "run_failing_command",
							"workdir": "/workspace",
						},
					},
					"error": "command failed: exit status 1",
					"data": map[string]any{
						"ok":    false,
						"tool":  "NexusDock__exec_command",
						"error": "command failed: exit status 1",
					},
				},
			}
			continue
		}

		events[i] = model.MessageEvent{
			ID:      fmt.Sprintf("event-%d", i),
			Kind:    "tool",
			Phase:   "done",
			CallKey: "chatdock_tool_call::{}",
			Text:    "调用完成：NexusDock__exec_command",
			Details: map[string]any{
				"event": "tool_call_result",
				"tool":  "chatdock_tool_call",
				"arguments": map[string]any{
					"tool": "NexusDock__exec_command",
					"arguments": map[string]any{
						"cmd":     cmdName,
						"workdir": "/workspace/chatdock",
					},
				},
				"data": map[string]any{
					"ok":   true,
					"tool": "NexusDock__exec_command",
					"result": map[string]any{
						"stdout": fmt.Sprintf("output of step %d\n", i),
					},
				},
			},
		}
	}

	trace := completedToolTrace(events)
	if tokens := EstimateTokens(trace); tokens > completedToolTraceMaxTokens {
		t.Fatalf("128 events trace tokens = %d, exceeds max %d", tokens, completedToolTraceMaxTokens)
	}
	if !strings.HasPrefix(trace, "<tool_execution_trace total=128>\n") {
		t.Fatalf("trace missing correct prefix header: %s", trace)
	}
	if !strings.HasSuffix(trace, "</tool_execution_trace>") {
		t.Fatalf("trace missing correct suffix footer: %s", trace)
	}
	if !strings.Contains(trace, "NexusDock__exec_command") {
		t.Fatalf("nested tool name was not extracted: %s", trace)
	}
	if !strings.Contains(trace, "step_0") {
		t.Fatalf("trace missing head events: %s", trace)
	}
	if !strings.Contains(trace, "step_127") {
		t.Fatalf("trace missing tail events: %s", trace)
	}
	if !strings.Contains(trace, "[error] NexusDock__exec_command") || !strings.Contains(trace, "exit status 1") {
		t.Fatalf("trace missing critical error event info: %s", trace)
	}
	if !strings.Contains(trace, "已折叠") {
		t.Fatalf("trace missing folding marker: %s", trace)
	}
}

func TestCompletedToolTraceExcludesMCPAppPresentationPayload(t *testing.T) {
	html := strings.Repeat("<section>app</section>", 2000)
	event := model.MessageEvent{
		Kind: "tool", Phase: "done", Text: "调用完成：demo__inspect",
		Details: map[string]any{
			"event":     "tool_call_result",
			"tool":      "demo__inspect",
			"arguments": map[string]any{"id": "res-123"},
			"data": map[string]any{
				"ok":            true,
				"tool":          "demo__inspect",
				"result":        map[string]any{"structuredContent": map[string]any{"answer": "kept"}},
				"mcp_app":       map[string]any{"resource_uri": "ui://demo/app", "html": html},
				"mcp_app_error": "presentation only",
			},
		},
	}
	entry, ok := completedToolTraceEntryForEvent(event)
	if !ok {
		t.Fatal("expected trace entry")
	}
	line := formatCompletedToolTraceEntry(entry)
	if strings.Contains(line, "<section>app</section>") || strings.Contains(line, "mcp_app") || strings.Contains(line, "presentation only") {
		t.Fatalf("presentation payload leaked into trace entry: %s", line)
	}
	if !strings.Contains(line, "demo__inspect") {
		t.Fatalf("tool name was lost: %s", line)
	}
}

func historicalToolTestMessage(messageID string, toolName string, before string, after string) model.Message {
	eventID := messageID + "-event"
	event := model.MessageEvent{
		ID:      eventID,
		Kind:    "tool",
		Phase:   "done",
		CallKey: toolName + `::{"path":"/tmp/demo"}`,
		Text:    "调用完成：" + toolName,
		Details: map[string]any{
			"event":     "tool_call_result",
			"tool":      toolName,
			"arguments": map[string]any{"path": "/tmp/demo"},
			"data": map[string]any{
				"ok":   true,
				"tool": toolName,
				"result": map[string]any{
					"value":                   toolName + " result",
					"_chatdock_model_content": "internal visual block",
				},
			},
		},
	}
	return model.Message{
		ID:        messageID,
		Role:      "assistant",
		Content:   before + after,
		Reasoning: "不得回传的推理",
		Events:    []model.MessageEvent{event},
		Parts: []model.MessagePart{
			{Kind: "reasoning", Text: "不得回传的推理"},
			{Kind: "text", Text: before},
			{Kind: "tool", CallKey: event.CallKey, Event: &model.MessageEvent{ID: eventID, Kind: "tool", Phase: "done", CallKey: event.CallKey, Text: event.Text}},
			{Kind: "text", Text: after},
		},
	}
}

func messagesWithRole(messages []map[string]any, role string) []map[string]any {
	matched := make([]map[string]any, 0)
	for _, message := range messages {
		if message["role"] == role {
			matched = append(matched, message)
		}
	}
	return matched
}
