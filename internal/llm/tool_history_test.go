package llm

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"chatdock/internal/model"
)

func TestBuildChatMessagesAnyRestoresOnlyTwoRecentToolMessages(t *testing.T) {
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
	if len(toolMessages) != 2 {
		t.Fatalf("restored tool message count = %d, want 2: %#v", len(toolMessages), messages)
	}
	if toolMessages[0]["name"] != "middle_tool" || toolMessages[1]["name"] != "newest_tool" {
		t.Fatalf("restored tools = %v, %v", toolMessages[0]["name"], toolMessages[1]["name"])
	}

	for _, toolMessage := range toolMessages {
		content, _ := toolMessage["content"].(string)
		if strings.Contains(content, "_chatdock_model_content") {
			t.Fatalf("internal model field leaked into history: %s", content)
		}
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "不得回传的推理") {
		t.Fatalf("reasoning leaked into model history: %s", raw)
	}

	for _, toolMessage := range toolMessages {
		messageIndex := indexOfMessage(messages, toolMessage)
		if messageIndex <= 0 {
			t.Fatalf("tool message has no assistant tool call before it: %#v", messages)
		}
		assistant := messages[messageIndex-1]
		calls, _ := assistant["tool_calls"].([]map[string]any)
		if len(calls) != 1 || calls[0]["id"] != toolMessage["tool_call_id"] {
			t.Fatalf("tool call pairing mismatch: assistant=%#v tool=%#v", assistant, toolMessage)
		}
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

func TestHistoricalToolContentTruncatesAtUTF8Boundary(t *testing.T) {
	content := historicalToolContent(map[string]any{"result": strings.Repeat("工具结果", 10000)})
	if len(content) > historicalToolResultMaxBytes {
		t.Fatalf("truncated content bytes = %d, max = %d", len(content), historicalToolResultMaxBytes)
	}
	if !utf8.ValidString(content) {
		t.Fatal("truncated content is not valid UTF-8")
	}
	if !strings.Contains(content, "历史工具结果过长") {
		t.Fatalf("truncation marker missing: %q", content)
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

func indexOfMessage(messages []map[string]any, target map[string]any) int {
	for index, message := range messages {
		if len(message) == len(target) && message["tool_call_id"] == target["tool_call_id"] && message["name"] == target["name"] {
			return index
		}
	}
	return -1
}
