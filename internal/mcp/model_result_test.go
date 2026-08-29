package mcp

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeCallToolResultForModelDropsDuplicateTextMirror(t *testing.T) {
	raw := map[string]any{
		"structuredContent": map[string]any{"answer": "ok"},
		"content": []any{
			map[string]any{"type": "text", "text": "{\n  \"answer\": \"ok\"\n}"},
		},
		"isError":   false,
		"extension": "keep-for-model",
		"_meta":     map[string]any{"ui": "full-result-only"},
	}

	got := NormalizeCallToolResultForModel(raw)
	want := map[string]any{
		"structuredContent": map[string]any{"answer": "ok"},
		"isError":           false,
		"extension":         "keep-for-model",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCallToolResultForModel() = %#v, want %#v", got, want)
	}
	if _, ok := raw["content"]; !ok {
		t.Fatal("normalization must not mutate the raw result")
	}
}

func TestNormalizeCallToolResultForModelPreservesDistinctTextContent(t *testing.T) {
	raw := map[string]any{
		"structuredContent": map[string]any{"answer": "ok"},
		"content": []any{
			map[string]any{"type": "text", "text": "human-readable explanation"},
		},
		"isError": false,
	}

	got, ok := NormalizeCallToolResultForModel(raw).(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", got)
	}
	if !reflect.DeepEqual(got["content"], raw["content"]) {
		t.Fatalf("distinct text content was lost: %#v", got)
	}
}

func TestNormalizeCallToolResultForModelBoundsLargeDistinctTextWithoutMutatingRaw(t *testing.T) {
	text := strings.Repeat("独立正文", CallToolMirrorVerifyMaxBytes)
	raw := map[string]any{
		"structuredContent": map[string]any{"summary": "metadata only"},
		"content": []any{map[string]any{
			"type": "text",
			"text": text,
		}},
		"isError": false,
	}

	got := NormalizeCallToolResultForModel(raw).(map[string]any)
	blocks := got["content"].([]any)
	preview := blocks[0].(map[string]any)["text"].(string)
	if len(preview) >= len(text) || !strings.Contains(preview, "ChatDock 已省略其余文本") {
		t.Fatalf("large distinct text was not reduced to a bounded preview: bytes=%d", len(preview))
	}
	omitted := got["_chatdock_content_omitted"].(map[string]any)
	if omitted["original_bytes"] != len(text) {
		t.Fatalf("omission metadata = %#v, want original_bytes=%d", omitted, len(text))
	}
	rawText := raw["content"].([]any)[0].(map[string]any)["text"].(string)
	if len(rawText) != len(text) {
		t.Fatal("normalization mutated the raw MCP text")
	}
}

func TestNormalizeCallToolResultForModelPreservesNonTextContent(t *testing.T) {
	raw := map[string]any{
		"structuredContent": map[string]any{"mime_type": "image/png"},
		"content": []any{
			map[string]any{"type": "text", "text": "preview"},
			map[string]any{"type": "image", "data": "aGVsbG8=", "mimeType": "image/png"},
		},
		"isError": false,
	}

	got, ok := NormalizeCallToolResultForModel(raw).(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", got)
	}
	if !reflect.DeepEqual(got["content"], raw["content"]) {
		t.Fatalf("non-text MCP content was lost: %#v", got)
	}
}

func TestNormalizeCallToolResultForModelPreservesErrorAndContentFallback(t *testing.T) {
	raw := map[string]any{
		"content": []any{map[string]any{"type": "text", "text": "remote failure"}},
		"isError": true,
	}
	got := NormalizeCallToolResultForModel(raw)
	if !reflect.DeepEqual(got, raw) {
		t.Fatalf("content-only CallToolResult changed unexpectedly: %#v", got)
	}
}

func TestNormalizeCallToolResultForModelLeavesBuiltinMapAlone(t *testing.T) {
	for name, raw := range map[string]map[string]any{
		"image metadata": {
			"url":                        "https://example.test/image.png",
			"_chatdock_model_content":    []any{map[string]any{"type": "image_url"}},
			"_chatdock_model_content_ok": true,
		},
		"plain content field": {
			"content": "ordinary builtin payload",
			"status":  "ok",
		},
		"structured without MCP content": {
			"structuredContent": map[string]any{"value": "ordinary payload"},
			"status":            "ok",
		},
		"content block without type": {
			"content": []any{map[string]any{"text": "ordinary payload"}},
			"status":  "ok",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := NormalizeCallToolResultForModel(raw); !reflect.DeepEqual(got, raw) {
				t.Fatalf("plain tool result must pass through unchanged: %#v", got)
			}
		})
	}
}
