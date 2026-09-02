package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"chatdock/internal/model"
)

const (
	completedToolTraceMaxTokens       = 2 * 1024
	completedToolTraceHeadEntries     = 6
	completedToolTraceTailEntries     = 18
	completedToolTraceLineMaxTokens   = 96
	completedToolTraceArgsMaxTokens   = 64
	completedToolTraceResultMaxTokens = 64
	completedToolTraceErrorMaxTokens  = 96
)

type completedToolTraceEntry struct {
	Tool   string
	Args   string
	Status string
	Result string
	Error  string
}

// historicalAssistantMessages 将已经完成的历史工具轮次投影为单条普通 assistant 文本。
// 历史轮次不再恢复 tool_calls / role=tool 协议结构，避免下一轮把几十到上百次调用
// 重新计入不可压缩的“最近一轮”。当前正在执行的 Active Turn 仍由 tools.go 保持原始配对。
func historicalAssistantMessages(item chatContextMessage) []map[string]any {
	trace := completedToolTrace(item.Events)
	content := strings.TrimSpace(item.Content)
	if trace != "" {
		if content == "" {
			content = trace
		} else {
			content = trace + "\n\n" + content
		}
	}
	if content == "" {
		return nil
	}
	return []map[string]any{{"role": "assistant", "content": content}}
}

func completedToolTrace(events []model.MessageEvent) string {
	entries := make([]completedToolTraceEntry, 0, len(events))
	for _, event := range events {
		entry, ok := completedToolTraceEntryForEvent(event)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return ""
	}

	prefix := fmt.Sprintf("<tool_execution_trace total=%d>\n", len(entries))
	footer := "</tool_execution_trace>"
	var body strings.Builder
	used := EstimateTokens(prefix) + EstimateTokens(footer)

	if counts := completedToolTraceCountLine(entries); counts != "" {
		if tokens := EstimateTokens(counts); used+tokens <= completedToolTraceMaxTokens {
			body.WriteString(counts)
			used += tokens
		}
	}

	selected := completedToolTraceIndexes(entries)
	skipped := len(entries) - len(selected)
	for _, index := range selected {
		line := formatCompletedToolTraceEntry(entries[index])
		tokens := EstimateTokens(line)
		if used+tokens > completedToolTraceMaxTokens {
			skipped++
			continue
		}
		body.WriteString(line)
		used += tokens
	}
	if skipped > 0 {
		line := fmt.Sprintf("- ... 已折叠 %d 条已完成工具事件 ...\n", skipped)
		if tokens := EstimateTokens(line); used+tokens <= completedToolTraceMaxTokens {
			body.WriteString(line)
		}
	}
	return prefix + body.String() + footer
}

func completedToolTraceEntryForEvent(event model.MessageEvent) (completedToolTraceEntry, bool) {
	if !isCompletedToolCallEvent(event) {
		return completedToolTraceEntry{}, false
	}
	data := mapFromAny(event.Details["data"])
	toolName := firstNonEmptyString(stringFromAny(event.Details["tool"]), stringFromAny(data["tool"]), toolNameFromEvent(event))
	arguments := mapFromAny(event.Details["arguments"])
	if len(arguments) == 0 {
		arguments = mapFromAny(data["arguments"])
	}
	if toolName == "chatdock_tool_call" {
		if nestedTool := stringFromAny(arguments["tool"]); nestedTool != "" {
			toolName = nestedTool
		}
		if nested := mapFromAny(arguments["arguments"]); len(nested) > 0 {
			arguments = nested
		} else if nested := mapFromJSONText(stringFromAny(arguments["arguments"])); len(nested) > 0 {
			arguments = nested
		}
	}
	if toolName == "" {
		toolName = "tool"
	}

	errorText := firstNonEmptyString(stringFromAny(event.Details["error"]), stringFromAny(data["error"]))
	status := "done"
	if event.Phase == "error" || errorText != "" {
		status = "error"
		if errorText == "" {
			errorText = strings.TrimSpace(event.Text)
		}
	}
	result := event.Details["result"]
	if result == nil {
		result = data["result"]
	}
	return completedToolTraceEntry{
		Tool:   compactTraceText(toolName, 48),
		Args:   compactTraceArguments(arguments),
		Status: status,
		Result: compactTraceResult(result),
		Error:  compactTraceText(errorText, completedToolTraceErrorMaxTokens),
	}, true
}

func toolNameFromEvent(event model.MessageEvent) string {
	if callKey := strings.TrimSpace(event.CallKey); callKey != "" {
		if split := strings.Index(callKey, "::"); split > 0 {
			return strings.TrimSpace(callKey[:split])
		}
	}
	for _, prefix := range []string{"调用完成：", "调用失败："} {
		if strings.HasPrefix(event.Text, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(event.Text, prefix))
		}
	}
	return ""
}

func mapFromJSONText(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil
	}
	return decoded
}

func compactTraceArguments(arguments map[string]any) string {
	if len(arguments) == 0 {
		return ""
	}
	priority := []string{"path", "cmd", "command", "query", "url", "resource", "resource_id", "id", "name", "action", "branch", "workdir", "cwd"}
	selected := make(map[string]any, 3)
	for _, key := range priority {
		value, ok := arguments[key]
		if !ok || len(selected) >= 3 {
			continue
		}
		if compact, ok := compactTraceArgumentValue(value); ok {
			selected[key] = compact
		}
	}
	if len(selected) == 0 {
		keys := make([]string, 0, len(arguments))
		for key := range arguments {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(lower, "api_key") {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if len(selected) >= 2 {
				break
			}
			if compact, ok := compactTraceArgumentValue(arguments[key]); ok {
				selected[key] = compact
			}
		}
	}
	if len(selected) == 0 {
		return ""
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		return ""
	}
	return compactTraceText(string(raw), completedToolTraceArgsMaxTokens)
}

func compactTraceResult(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text != "" && EstimateTokens(text) <= completedToolTraceResultMaxTokens {
			return text
		}
		return ""
	}
	result := mapFromAny(value)
	if len(result) == 0 {
		switch value.(type) {
		case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return fmt.Sprint(value)
		}
		return ""
	}

	priority := []string{"id", "status", "state", "path", "url", "branch", "commit", "sha", "exit_code", "count", "total", "ok", "command_ok", "message", "summary", "stdout"}
	selected := make(map[string]any, 4)
	for _, key := range priority {
		if len(selected) >= 4 {
			break
		}
		item, ok := result[key]
		if !ok {
			continue
		}
		compact, ok := compactTraceResultValue(item)
		if ok {
			selected[key] = compact
		}
	}
	if len(selected) == 0 {
		return ""
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		return ""
	}
	return compactTraceText(string(raw), completedToolTraceResultMaxTokens)
}

func compactTraceResultValue(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || EstimateTokens(text) > completedToolTraceResultMaxTokens/2 {
			return nil, false
		}
		return text, true
	case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed, true
	default:
		return nil, false
	}
}

func compactTraceArgumentValue(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		text := compactTraceText(typed, 48)
		return text, text != ""
	case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed, true
	default:
		return nil, false
	}
}

func completedToolTraceCountLine(entries []completedToolTraceEntry) string {
	counts := make(map[string]int)
	for _, entry := range entries {
		counts[entry.Tool]++
	}
	type countEntry struct {
		name  string
		count int
	}
	ordered := make([]countEntry, 0, len(counts))
	for name, count := range counts {
		ordered = append(ordered, countEntry{name: name, count: count})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].count != ordered[j].count {
			return ordered[i].count > ordered[j].count
		}
		return ordered[i].name < ordered[j].name
	})
	limit := minInt(len(ordered), 8)
	parts := make([]string, 0, limit+1)
	for _, item := range ordered[:limit] {
		parts = append(parts, fmt.Sprintf("%s×%d", item.name, item.count))
	}
	if len(ordered) > limit {
		otherCalls := 0
		for _, item := range ordered[limit:] {
			otherCalls += item.count
		}
		parts = append(parts, fmt.Sprintf("其他调用×%d", otherCalls))
	}
	return "tools: " + strings.Join(parts, ", ") + "\n"
}

func completedToolTraceIndexes(entries []completedToolTraceEntry) []int {
	if len(entries) <= completedToolTraceHeadEntries+completedToolTraceTailEntries {
		indexes := make([]int, len(entries))
		for index := range entries {
			indexes[index] = index
		}
		return indexes
	}
	selected := make(map[int]bool, completedToolTraceHeadEntries+completedToolTraceTailEntries)
	for index := 0; index < completedToolTraceHeadEntries; index++ {
		selected[index] = true
	}
	for index := len(entries) - completedToolTraceTailEntries; index < len(entries); index++ {
		selected[index] = true
	}
	for index, entry := range entries {
		if entry.Status == "error" {
			selected[index] = true
		}
	}
	indexes := make([]int, 0, len(selected))
	for index := range selected {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}

func formatCompletedToolTraceEntry(entry completedToolTraceEntry) string {
	line := fmt.Sprintf("- [%s] %s", entry.Status, entry.Tool)
	if entry.Args != "" {
		line += " " + entry.Args
	}
	if entry.Error != "" {
		line += " -> " + entry.Error
	} else if entry.Result != "" {
		line += " => " + entry.Result
	}
	return compactTraceText(line, completedToolTraceLineMaxTokens) + "\n"
}

func compactTraceText(value string, maxTokens int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxTokens <= 0 || EstimateTokens(value) <= maxTokens {
		return value
	}
	runes := []rune(value)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		candidate := strings.TrimSpace(string(runes[:mid])) + "…"
		if EstimateTokens(candidate) <= maxTokens {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low <= 0 {
		return "…"
	}
	return strings.TrimSpace(string(runes[:low])) + "…"
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
