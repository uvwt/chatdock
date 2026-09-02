package llm

import (
	"strings"

	"chatdock/internal/model"
)

const historicalToolMessageLimit = 2

// HistoricalToolMessageIndexes 返回最近两个包含真实工具结果的助手消息索引。
// Store 用它按需补载事件详情，LLM 上下文也用同一规则决定哪些完成轮次需要生成紧凑 trace。
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
