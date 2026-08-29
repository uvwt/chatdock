package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"chatdock/internal/mcp"
)

func TestModelToolContentNormalizesSmallMCPMirror(t *testing.T) {
	payload := map[string]any{
		"ok":   true,
		"tool": "demo",
		"result": map[string]any{
			"structuredContent": map[string]any{"value": "small"},
			"content": []any{map[string]any{
				"type": "text",
				"text": `{"value":"small"}`,
			}},
			"isError": false,
		},
	}

	got := modelToolContent(payload, currentToolResultMaxBytes, "工具结果过长")
	if strings.Contains(got, `"content"`) {
		t.Fatal("duplicate text mirror must not be sent to the model")
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("model tool content must be valid JSON: %q", got)
	}
}

func TestModelToolContentBoundsLargeMCPResultBeforeSerialization(t *testing.T) {
	value := strings.Repeat("界", 8<<20)
	payload := map[string]any{
		"ok":   true,
		"tool": "demo",
		"result": map[string]any{
			"structuredContent": map[string]any{"value": value},
			"content": []any{map[string]any{
				"type": "text",
				"text": `{"value":"` + value + `"}`,
			}},
			"isError": false,
		},
	}

	got := modelToolContent(payload, currentToolResultMaxBytes, "工具结果过长")
	if len(got) > currentToolResultMaxBytes {
		t.Fatalf("bounded content has %d bytes, max %d", len(got), currentToolResultMaxBytes)
	}
	if !utf8.ValidString(got) || !json.Valid([]byte(got)) {
		t.Fatalf("bounded tool content must stay valid UTF-8 JSON: %q", got)
	}
	if !strings.Contains(got, "已截断") {
		t.Fatalf("truncation marker missing: %q", got)
	}
	if !strings.Contains(got, `"structuredContent"`) || !strings.Contains(got, `"_chatdock_content_omitted"`) {
		t.Fatalf("large MCP result must prioritize structuredContent and mark the bounded text preview: %q", got)
	}
	if strings.Contains(got, strings.Repeat("界", 4096)) {
		t.Fatal("large MCP text mirror leaked into model content instead of a bounded preview")
	}
	result := payload["result"].(map[string]any)
	if len(result["structuredContent"].(map[string]any)["value"].(string)) != len(value) {
		t.Fatal("model-side shrinking must not mutate the raw structuredContent")
	}
	rawText := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if len(rawText) <= mcp.CallToolMirrorVerifyMaxBytes {
		t.Fatal("model-side normalization must not mutate the raw MCP content")
	}
}

func TestModelToolContentShrinksTypedStructSlice(t *testing.T) {
	type provider struct {
		ID     string   `json:"id"`
		Models []string `json:"models"`
	}
	providers := make([]provider, 80)
	for index := range providers {
		providers[index] = provider{ID: strings.Repeat("provider", 200), Models: []string{strings.Repeat("model", 200)}}
	}

	got := modelToolContent(map[string]any{"ok": true, "tool": "providers", "result": map[string]any{"providers": providers}}, currentToolResultMaxBytes, "工具结果过长")
	if !json.Valid([]byte(got)) || len(got) > currentToolResultMaxBytes {
		t.Fatalf("typed struct slice must produce bounded valid JSON: bytes=%d content=%q", len(got), got)
	}
	if !strings.Contains(got, `"providers"`) || !strings.Contains(got, `"id"`) || strings.Contains(got, "unsupported type") {
		t.Fatalf("typed struct data was not meaningfully preserved: %q", got)
	}
}

func TestModelToolContentKeepsUsefulEnvelopeForKeyDenseResult(t *testing.T) {
	rows := make([]any, 32)
	for rowIndex := range rows {
		row := make(map[string]any, 32)
		for fieldIndex := 0; fieldIndex < 32; fieldIndex++ {
			row[fmt.Sprintf("field_%02d_long", fieldIndex)] = fmt.Sprintf("value_%02d_%02d", rowIndex, fieldIndex)
		}
		rows[rowIndex] = row
	}

	got := modelToolContent(map[string]any{
		"ok":   true,
		"tool": "dense_query",
		"result": map[string]any{
			"structuredContent": map[string]any{"rows": rows},
		},
	}, currentToolResultMaxBytes, "工具结果过长")

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("key-dense result must remain valid JSON: %v\n%s", err, got)
	}
	if decoded["tool"] != "dense_query" || decoded["ok"] != true {
		t.Fatalf("key-dense result lost tool envelope: %#v", decoded)
	}
	if _, ok := decoded["result"]; !ok {
		t.Fatalf("key-dense result fell back to an anonymous omission stub: %#v", decoded)
	}
}

func TestModelToolContentHandlesHTMLEscapingWithinBudget(t *testing.T) {
	html := strings.Repeat("<tag>&value</tag>", 1000)
	got := modelToolContent(map[string]any{
		"ok":   true,
		"tool": "read_page",
		"result": map[string]any{
			"html": html,
			"text": html,
		},
	}, currentToolResultMaxBytes, "工具结果过长")

	if len(got) > currentToolResultMaxBytes || !json.Valid([]byte(got)) {
		t.Fatalf("HTML-heavy result must remain bounded valid JSON: bytes=%d content=%q", len(got), got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["tool"] != "read_page" {
		t.Fatalf("HTML-heavy result lost the tool envelope: %#v", decoded)
	}
	if _, ok := decoded["result"]; !ok {
		t.Fatalf("HTML-heavy result was reduced to a whole-payload stub: %#v", decoded)
	}
}

func TestRebalanceCurrentToolContentStubsOldestAndPreservesProtocolFields(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "history"},
		{"role": "assistant", "tool_calls": []any{"call_1"}},
		{"role": "tool", "tool_call_id": "call_1", "name": "first", "content": strings.Repeat("a", 80)},
		{"role": "assistant", "tool_calls": []any{"call_2"}},
		{"role": "tool", "tool_call_id": "call_2", "name": "second", "content": strings.Repeat("b", 80)},
	}

	rebalanceToolContent(messages, 1, 120)

	first := messages[2]
	second := messages[4]
	if first["tool_call_id"] != "call_1" || first["name"] != "first" || first["role"] != "tool" {
		t.Fatalf("first tool protocol fields changed: %#v", first)
	}
	if second["content"] != strings.Repeat("b", 80) {
		t.Fatalf("newest tool content should be retained: %#v", second)
	}
	if !strings.Contains(first["content"].(string), "tool_context_budget") {
		t.Fatalf("oldest tool content was not replaced by an omission stub: %#v", first)
	}
}

func TestRebalanceCurrentToolContentDoesNotTouchHistoricalToolMessages(t *testing.T) {
	historical := strings.Repeat("h", 200)
	messages := []map[string]any{
		{"role": "tool", "tool_call_id": "history", "name": "old", "content": historical},
		{"role": "tool", "tool_call_id": "current", "name": "new", "content": strings.Repeat("n", 200)},
	}

	rebalanceToolContent(messages, 1, 100)
	if messages[0]["content"] != historical {
		t.Fatal("aggregate budget must not rewrite historical messages")
	}
	if !strings.Contains(messages[1]["content"].(string), "tool_context_budget") {
		t.Fatal("current tool message should be budgeted")
	}
}
