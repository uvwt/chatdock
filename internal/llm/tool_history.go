package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
)

const (
	historicalToolMessageLimit   = 2
	historicalToolResultMaxBytes = 16 << 10
)

// HistoricalToolMessageIndexes 返回最近两个包含真实工具结果的助手消息索引。
// Store 用它按需补载事件详情，LLM 上下文也用同一规则决定哪些轮次需要恢复。
func HistoricalToolMessageIndexes(history []model.Message) []int {
	indexes := make([]int, 0, historicalToolMessageLimit)
	for index := len(history) - 1; index >= 0 && len(indexes) < historicalToolMessageLimit; index-- {
		if history[index].Role != "assistant" || !messageHasCompletedToolEvent(history[index]) {
			continue
		}
		indexes = append(indexes, index)
	}
	for left, right := 0, len(indexes)-1; left < right; left, right = left+1, right-1 {
		indexes[left], indexes[right] = indexes[right], indexes[left]
	}
	return indexes
}

func historicalToolMessageIndexSet(history []model.Message) map[int]bool {
	selected := make(map[int]bool, historicalToolMessageLimit)
	for _, index := range HistoricalToolMessageIndexes(history) {
		selected[index] = true
	}
	return selected
}

func messageHasCompletedToolEvent(message model.Message) bool {
	for _, event := range message.Events {
		if isCompletedToolCallEvent(event) {
			return true
		}
	}
	return false
}

func isCompletedToolCallEvent(event model.MessageEvent) bool {
	if event.Kind != "tool" || (event.Phase != "done" && event.Phase != "error") {
		return false
	}
	if eventName := strings.TrimSpace(stringFromAny(event.Details["event"])); eventName != "" {
		return eventName == "tool_call_result"
	}
	// 重启后事件详情保持懒加载，先通过持久化的展示文本识别真实工具结果。
	return strings.HasPrefix(event.Text, "调用完成：") || strings.HasPrefix(event.Text, "调用失败：")
}

func historicalAssistantMessages(item chatContextMessage, contextIndex int) []map[string]any {
	messages := make([]map[string]any, 0)
	toolCount := 0
	for _, event := range item.Events {
		call, payload, ok := historicalToolCall(event, contextIndex, toolCount)
		if !ok {
			continue
		}
		messages = appendHistoricalToolExchange(messages, call, payload)
		toolCount++
	}
	if toolCount == 0 {
		return nil
	}
	if finalText := strings.TrimSpace(item.Content); finalText != "" {
		messages = append(messages, map[string]any{"role": "assistant", "content": finalText})
	}
	return messages
}

func appendHistoricalToolExchange(messages []map[string]any, call ModelToolCall, payload map[string]any) []map[string]any {
	messages = append(messages, map[string]any{
		"role":       "assistant",
		"content":    "",
		"tool_calls": encodeModelToolCalls([]ModelToolCall{call}),
	})
	messages = append(messages, map[string]any{
		"role":         "tool",
		"tool_call_id": call.ID,
		"name":         call.Function.Name,
		"content":      historicalToolContent(payload),
	})
	return messages
}

func historicalToolCall(event model.MessageEvent, contextIndex int, toolIndex int) (ModelToolCall, map[string]any, bool) {
	if !isCompletedToolCallEvent(event) || len(event.Details) == 0 {
		return ModelToolCall{}, nil, false
	}
	data := mapFromAny(event.Details["data"])
	toolName := firstNonEmptyString(stringFromAny(event.Details["tool"]), stringFromAny(data["tool"]))
	if toolName == "" {
		return ModelToolCall{}, nil, false
	}
	arguments := mapFromAny(event.Details["arguments"])
	if len(arguments) == 0 {
		arguments = mapFromAny(data["arguments"])
	}

	payload := modelHistoryPayload(data)
	if len(payload) == 0 {
		payload = map[string]any{
			"ok":     event.Phase == "done",
			"tool":   toolName,
			"result": event.Details["result"],
		}
		if errorText := strings.TrimSpace(stringFromAny(event.Details["error"])); errorText != "" {
			payload["error"] = errorText
		}
	}
	payload = sanitizeToolPayload(payload)
	callID := fmt.Sprintf("call_history_%d_%d", contextIndex, toolIndex)
	return ModelToolCall{
		ID:   callID,
		Type: "function",
		Function: ModelToolCallFunc{
			Name:      toolName,
			Arguments: mcp.CompactJSON(arguments),
		},
	}, payload, true
}

func historicalToolContent(payload map[string]any) string {
	return modelToolContent(payload, historicalToolResultMaxBytes, "历史工具结果过长")
}

func utf8Prefix(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}

func utf8Suffix(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	start := len(value) - maxBytes
	for start < len(value) && !utf8.ValidString(value[start:]) {
		start++
	}
	return value[start:]
}

func modelHistoryPayload(data map[string]any) map[string]any {
	if len(data) == 0 {
		return nil
	}
	out := make(map[string]any, len(data))
	for key, value := range data {
		switch key {
		case "mcp_app", "mcp_app_error":
			// MCP App HTML and presentation errors belong to the host UI only.
			// They must never consume model-history budget or displace real tool output.
			continue
		default:
			out[key] = value
		}
	}
	return out
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
