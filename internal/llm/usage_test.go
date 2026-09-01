package llm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeUsageOpenAIAndDeepSeekFields(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		input     int
		output    int
		reasoning int
		hit       int
		miss      int
		total     int
	}{
		{
			name:    "openai details",
			payload: `{"usage":{"prompt_tokens":1000,"completion_tokens":500,"total_tokens":1500,"prompt_tokens_details":{"cached_tokens":240},"completion_tokens_details":{"reasoning_tokens":80}}}`,
			input:   1000, output: 500, reasoning: 80, hit: 240, miss: 760, total: 1500,
		},
		{
			name:    "deepseek aliases",
			payload: `{"prompt_tokens":2000,"completion_tokens":800,"reasoning_tokens":300,"prompt_cache_hit_tokens":400,"prompt_cache_miss_tokens":1600}`,
			input:   2000, output: 800, reasoning: 300, hit: 400, miss: 1600, total: 3100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tt.payload), &envelope); err != nil {
				t.Fatal(err)
			}
			raw := envelope["usage"]
			if len(raw) == 0 {
				raw = json.RawMessage(tt.payload)
			}
			usage := normalizeUsage(raw)
			if usage == nil || usage.InputTokens != tt.input || usage.OutputTokens != tt.output || usage.ReasoningTokens != tt.reasoning || usage.CacheHitTokens != tt.hit || usage.CacheMissTokens != tt.miss || usage.TotalTokens != tt.total || usage.Source != "provider" {
				t.Fatalf("usage = %#v", usage)
			}
		})
	}
}

func TestNormalizeUsageMissingAndAccumulatedUsage(t *testing.T) {
	if got := normalizeUsage(json.RawMessage(`{"usage":{}}`)); got != nil {
		t.Fatalf("empty provider usage = %#v, want nil", got)
	}
	first := normalizeUsage(json.RawMessage(`{"input_tokens":100,"output_tokens":20,"reasoning_tokens":5}`))
	second := normalizeUsage(json.RawMessage(`{"input_tokens":200,"output_tokens":30,"reasoning_tokens":10,"cache_hit_tokens":80}`))
	if first == nil || second == nil {
		t.Fatal("expected both usage records")
	}
	first.Add(*second)
	if first.TotalTokens != 365 || first.InputTokens != 300 || first.ReasoningTokens != 15 || first.CacheHitTokens != 80 || first.CacheMissTokens != 220 {
		t.Fatalf("accumulated usage = %#v", first)
	}
}
