package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/model"
)

// normalizeUsage 兼容 OpenAI、DeepSeek 以及常见 OpenAI-compatible 网关的 usage 字段。
// 供应商没有返回可识别字段时返回 nil，调用方必须把它显示为“供应商未提供”。
func normalizeUsage(raw json.RawMessage) *model.Usage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	input := firstUsageInt(payload, "prompt_tokens", "input_tokens")
	output := firstUsageInt(payload, "completion_tokens", "output_tokens")
	reasoning := firstUsageInt(payload, "reasoning_tokens")
	for _, key := range []string{"completion_tokens_details", "output_tokens_details", "output_token_details"} {
		reasoning = maxInt(reasoning, firstUsageInt(usageMapValue(payload[key]), "reasoning_tokens"))
	}
	hit := firstUsageInt(payload, "prompt_cache_hit_tokens", "cache_read_input_tokens", "cached_tokens", "cache_hit_tokens")
	for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
		hit = maxInt(hit, firstUsageInt(usageMapValue(payload[key]), "cached_tokens", "cache_read_input_tokens"))
	}
	miss := firstUsageInt(payload, "prompt_cache_miss_tokens", "cache_write_input_tokens", "cache_creation_input_tokens", "cache_miss_tokens")
	total := firstUsageInt(payload, "total_tokens")
	if input == 0 && output == 0 && reasoning == 0 && hit == 0 && miss == 0 && total == 0 {
		return nil
	}
	if miss == 0 && input > hit {
		miss = input - hit
	}
	if total == 0 {
		total = input + output + reasoning
	}
	return &model.Usage{
		InputTokens:     input,
		OutputTokens:    output,
		ReasoningTokens: reasoning,
		CacheHitTokens:  hit,
		CacheMissTokens: miss,
		TotalTokens:     total,
		Source:          "provider",
	}
}

func usageMapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func firstUsageInt(payload map[string]any, keys ...string) int {
	for _, key := range keys {
		if value := usageInt(payload[key]); value > 0 {
			return value
		}
	}
	return 0
}

func usageInt(value any) int {
	switch typed := value.(type) {
	case int:
		return maxInt(typed, 0)
	case int64:
		return maxInt(int(typed), 0)
	case float64:
		return maxInt(int(typed), 0)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return maxInt(int(value), 0)
		}
	case string:
		var value int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &value); err == nil {
			return maxInt(value, 0)
		}
	}
	return 0
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
