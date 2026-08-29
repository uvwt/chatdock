package llm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"chatdock/internal/mcp"
)

const (
	currentToolResultMaxBytes       = 24 << 10
	currentToolAggregateMaxBytes    = 96 << 10
	historicalToolAggregateMaxBytes = 48 << 10
	toolValueMaxStringBytes         = 4 << 10
	toolValueMaxCollectionItems     = 32
	toolValueMaxMapKeyBytes         = 256
	toolValueMaxDepth               = 24
)

var timeValueType = reflect.TypeOf(time.Time{})

func modelToolContent(payload map[string]any, maxBytes int, markerLabel string) string {
	if maxBytes <= 0 {
		return mcp.CompactJSON(payload)
	}

	// 原始 payload 继续用于事件持久化/UI；这里只创建模型侧浅副本。
	// MCP envelope 先去掉可确认的文本镜像或压成小预览，再做树级预算，避免多 MB 副本进入后续序列化。
	modelPayload := make(map[string]any, len(payload))
	for key, value := range payload {
		modelPayload[key] = value
	}
	if result, ok := modelPayload["result"]; ok {
		modelPayload["result"] = mcp.NormalizeCallToolResultForModel(result)
	}

	budget := maxBytes * 3 / 4
	shrunk, _ := shrinkToolValue(modelPayload, &budget, 0).(map[string]any)
	content := mcp.CompactJSON(shrunk)
	return boundedToolJSON(shrunk, content, maxBytes, markerLabel)
}

func shrinkToolValue(value any, budget *int, depth int) any {
	if budget == nil || *budget <= 0 {
		return nil
	}
	if depth >= toolValueMaxDepth {
		return shrinkToolString("[tool value omitted: max depth reached]", budget)
	}

	switch item := value.(type) {
	case string:
		return shrinkToolString(item, budget)
	case map[string]any:
		return shrinkToolMap(item, budget, depth)
	case []any:
		return shrinkToolArray(item, budget, depth)
	case []map[string]any:
		return shrinkToolMapArray(item, budget, depth)
	case []byte:
		return shrinkToolString(fmt.Sprintf("[binary tool value: original_bytes=%d]", len(item)), budget)
	case json.RawMessage:
		return shrinkToolString(fmt.Sprintf("[raw JSON tool value: original_bytes=%d]", len(item)), budget)
	case time.Time:
		return shrinkToolString(item.Format(time.RFC3339Nano), budget)
	case nil, bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return shrinkToolScalar(item, budget)
	default:
		return shrinkReflectedToolValue(reflect.ValueOf(value), budget, depth)
	}
}

func shrinkToolMap(item map[string]any, budget *int, depth int) map[string]any {
	out := make(map[string]any)
	if !consumeToolBudget(budget, 2) { // {}
		return out
	}

	emitted := 0
	for _, key := range orderedToolMapKeys(item) {
		if key == "_meta" || emitted >= toolValueMaxCollectionItems {
			continue
		}
		if len(key) > toolValueMaxMapKeyBytes {
			continue
		}
		keyCost := encodedToolStringBytes(key) + 1 // key + ':'
		if emitted > 0 {
			keyCost++ // ','
		}
		// 至少给值留出一个 null 的空间；装不下时跳过该字段，避免生成半截 JSON 语义。
		if *budget < keyCost+4 {
			continue
		}
		consumeToolBudget(budget, keyCost)
		out[key] = shrinkToolValue(item[key], budget, depth+1)
		emitted++
	}

	if omitted := len(item) - emitted; omitted > 0 {
		appendToolOmittedFields(out, omitted, budget, depth)
	}
	return out
}

func orderedToolMapKeys(item map[string]any) []string {
	priority := []string{
		"ok", "error", "tool", "result",
		"_chatdock_content_omitted", "structuredContent", "isError", "content",
	}
	seen := make(map[string]struct{}, len(priority))
	keys := make([]string, 0, toolValueMaxCollectionItems*2)
	for _, key := range priority {
		if _, ok := item[key]; ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}

	// 大 map 不为排序复制全部 key，只取有限候选；普通小 map 保持稳定排序，便于测试与排查。
	remaining := make([]string, 0, toolValueMaxCollectionItems*2)
	for key := range item {
		if _, ok := seen[key]; ok || key == "_meta" {
			continue
		}
		remaining = append(remaining, key)
		if len(item) > toolValueMaxCollectionItems*4 && len(remaining) >= toolValueMaxCollectionItems*2 {
			break
		}
	}
	sort.Strings(remaining)
	keys = append(keys, remaining...)
	return keys
}

func appendToolOmittedFields(out map[string]any, omitted int, budget *int, depth int) {
	if omitted <= 0 || *budget < 48 {
		return
	}
	markerKey := "_chatdock_omitted_fields"
	keyCost := encodedToolStringBytes(markerKey) + 1
	if len(out) > 0 {
		keyCost++
	}
	if *budget < keyCost+4 {
		return
	}
	consumeToolBudget(budget, keyCost)
	out[markerKey] = shrinkToolScalar(omitted, budget)
}

func shrinkToolArray(item []any, budget *int, depth int) []any {
	out := make([]any, 0, minInt(len(item), toolValueMaxCollectionItems)+1)
	if !consumeToolBudget(budget, 2) { // []
		return out
	}
	limit := minInt(len(item), toolValueMaxCollectionItems)
	for index := 0; index < limit; index++ {
		comma := 0
		if len(out) > 0 {
			comma = 1
		}
		if *budget < comma+4 {
			break
		}
		consumeToolBudget(budget, comma)
		out = append(out, shrinkToolValue(item[index], budget, depth+1))
	}
	return appendToolArrayOmission(out, len(item)-len(out), budget, depth)
}

func shrinkToolMapArray(item []map[string]any, budget *int, depth int) []any {
	out := make([]any, 0, minInt(len(item), toolValueMaxCollectionItems)+1)
	if !consumeToolBudget(budget, 2) {
		return out
	}
	limit := minInt(len(item), toolValueMaxCollectionItems)
	for index := 0; index < limit; index++ {
		comma := 0
		if len(out) > 0 {
			comma = 1
		}
		if *budget < comma+4 {
			break
		}
		consumeToolBudget(budget, comma)
		out = append(out, shrinkToolMap(item[index], budget, depth+1))
	}
	return appendToolArrayOmission(out, len(item)-len(out), budget, depth)
}

func appendToolArrayOmission(out []any, omitted int, budget *int, depth int) []any {
	if omitted <= 0 || *budget < 64 {
		return out
	}
	if len(out) > 0 {
		consumeToolBudget(budget, 1)
	}
	marker := map[string]any{"_chatdock_omitted_items": omitted}
	return append(out, shrinkToolMap(marker, budget, depth+1))
}

func shrinkReflectedToolValue(value reflect.Value, budget *int, depth int) any {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return shrinkToolScalar(nil, budget)
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return shrinkToolScalar(nil, budget)
	}
	if value.Type() == timeValueType && value.CanInterface() {
		return shrinkToolString(value.Interface().(time.Time).Format(time.RFC3339Nano), budget)
	}

	switch value.Kind() {
	case reflect.Struct:
		fields := make(map[string]any)
		typeInfo := value.Type()
		for index := 0; index < value.NumField(); index++ {
			fieldInfo := typeInfo.Field(index)
			fieldValue := value.Field(index)
			if fieldInfo.PkgPath != "" || !fieldValue.CanInterface() {
				continue
			}
			name, omitEmpty, skip := jsonToolFieldName(fieldInfo)
			if skip || name == "" || omitEmpty && fieldValue.IsZero() {
				continue
			}
			fields[name] = fieldValue.Interface()
		}
		return shrinkToolMap(fields, budget, depth)
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return shrinkToolString(fmt.Sprintf("[binary tool value: original_bytes=%d]", value.Len()), budget)
		}
		out := make([]any, 0, minInt(value.Len(), toolValueMaxCollectionItems)+1)
		if !consumeToolBudget(budget, 2) {
			return out
		}
		limit := minInt(value.Len(), toolValueMaxCollectionItems)
		for index := 0; index < limit; index++ {
			comma := 0
			if len(out) > 0 {
				comma = 1
			}
			if *budget < comma+4 {
				break
			}
			consumeToolBudget(budget, comma)
			out = append(out, shrinkReflectedToolValue(value.Index(index), budget, depth+1))
		}
		return appendToolArrayOmission(out, value.Len()-len(out), budget, depth)
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return shrinkToolString(fmt.Sprintf("[tool map omitted: unsupported key type %s]", value.Type().Key()), budget)
		}
		fields := make(map[string]any)
		iter := value.MapRange()
		for iter.Next() && len(fields) < toolValueMaxCollectionItems*2 {
			key := iter.Key().String()
			entry := iter.Value()
			if entry.IsValid() && entry.CanInterface() {
				fields[key] = entry.Interface()
			}
		}
		if value.Len() > len(fields) {
			fields["_chatdock_omitted_fields"] = value.Len() - len(fields)
		}
		return shrinkToolMap(fields, budget, depth)
	case reflect.String:
		return shrinkToolString(value.String(), budget)
	case reflect.Bool:
		return shrinkToolScalar(value.Bool(), budget)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return shrinkToolScalar(value.Int(), budget)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return shrinkToolScalar(value.Uint(), budget)
	case reflect.Float32, reflect.Float64:
		return shrinkToolScalar(value.Float(), budget)
	default:
		return shrinkToolString(fmt.Sprintf("[tool value omitted: unsupported type %s]", value.Type()), budget)
	}
}

func jsonToolFieldName(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = field.Name
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
			break
		}
	}
	return name, omitEmpty, false
}

func shrinkToolScalar(value any, budget *int) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return shrinkToolString(fmt.Sprintf("[tool value omitted: %T is not JSON encodable]", value), budget)
	}
	if budget == nil || *budget < len(raw) {
		if budget != nil {
			consumeToolBudget(budget, minInt(4, *budget))
		}
		return nil
	}
	consumeToolBudget(budget, len(raw))
	return value
}

func shrinkToolString(value string, budget *int) string {
	if budget == nil || *budget <= 2 {
		return ""
	}

	prefix := value
	if len(prefix) > toolValueMaxStringBytes {
		prefix = utf8Prefix(prefix, toolValueMaxStringBytes)
	}
	truncated := len(prefix) < len(value)

	for {
		candidate := prefix
		if truncated {
			candidate += fmt.Sprintf("...[工具字段已截断: original_bytes=%d]", len(value))
		}
		encoded, _ := json.Marshal(candidate)
		if len(encoded) <= *budget {
			consumeToolBudget(budget, len(encoded))
			return candidate
		}
		if len(prefix) == 0 {
			fallback := "[tool string omitted]"
			encoded, _ = json.Marshal(fallback)
			consumeToolBudget(budget, len(encoded))
			return fallback
		}
		prefix = utf8Prefix(prefix, len(prefix)/2)
		truncated = true
	}
}

func encodedToolStringBytes(value string) int {
	raw, _ := json.Marshal(value)
	return len(raw)
}

func consumeToolBudget(budget *int, size int) bool {
	if budget == nil || size <= 0 {
		return true
	}
	if *budget < size {
		*budget = 0
		return false
	}
	*budget -= size
	return true
}

func boundedToolJSON(payload map[string]any, content string, maxBytes int, markerLabel string) string {
	if maxBytes <= 0 || len(content) <= maxBytes {
		return content
	}

	// 正常路径已按 JSON 成本收缩；若仍因极端键名/类型开销越界，兜底至少保留工具名、成功状态和短错误。
	fallback := map[string]any{
		"omitted": map[string]any{
			"reason":           markerLabel,
			"serialized_bytes": len(content),
			"message":          "工具结果过长，已截断到上下文预算内",
		},
	}
	if tool, ok := payload["tool"]; ok {
		fallback["tool"] = tool
	}
	if success, ok := payload["ok"]; ok {
		fallback["ok"] = success
	}
	if toolErr, ok := payload["error"]; ok {
		errorBudget := minInt(1024, maxBytes/4)
		fallback["error"] = shrinkToolValue(toolErr, &errorBudget, 0)
	}
	raw := mcp.CompactJSON(fallback)
	if len(raw) <= maxBytes {
		return raw
	}
	return mcp.CompactJSON(map[string]any{"omitted": map[string]any{"reason": markerLabel}})
}

func rebalanceToolContent(messages []map[string]any, startIndex int, maxBytes int) {
	if maxBytes <= 0 || startIndex >= len(messages) {
		return
	}
	if startIndex < 0 {
		startIndex = 0
	}

	total := 0
	toolIndexes := make([]int, 0)
	for index := startIndex; index < len(messages); index++ {
		if messages[index]["role"] != "tool" {
			continue
		}
		content, _ := messages[index]["content"].(string)
		total += len(content)
		toolIndexes = append(toolIndexes, index)
	}
	if total <= maxBytes {
		return
	}

	for _, index := range toolIndexes {
		if total <= maxBytes {
			break
		}
		content, _ := messages[index]["content"].(string)
		stub := omittedToolContent(messages[index], len(content))
		if len(stub) >= len(content) {
			continue
		}
		messages[index]["content"] = stub
		total -= len(content) - len(stub)
	}
}

func omittedToolContent(message map[string]any, originalBytes int) string {
	return mcp.CompactJSON(map[string]any{
		"tool": message["name"],
		"omitted": map[string]any{
			"reason":         "tool_context_budget",
			"original_bytes": originalBytes,
		},
	})
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
