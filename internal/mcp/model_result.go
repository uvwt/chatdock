package mcp

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"unicode/utf8"
)

const (
	CallToolMirrorVerifyMaxBytes  = 64 << 10
	callToolLargeTextPreviewBytes = 512
)

// NormalizeCallToolResultForModel returns a smaller model-facing projection of an MCP
// CallToolResult while leaving the raw result untouched for persistence and UI.
// Plain ChatDock/builtin tool results are not MCP envelopes and are returned unchanged.
func NormalizeCallToolResultForModel(value any) any {
	result, ok := value.(map[string]any)
	if !ok || !looksLikeCallToolResult(result) {
		return value
	}

	// 采用减法而不是白名单重建：MCP server 可能带扩展字段，模型侧不应因为
	// ChatDock 识别了 CallToolResult 就静默丢失未知但有意义的数据。
	normalized := maps.Clone(result)
	delete(normalized, "_meta")
	structured, hasStructured := result["structuredContent"]
	if !hasStructured {
		return normalized
	}

	blocks, ok := callToolContentBlocks(result["content"])
	if !ok || len(blocks) != 1 || strings.TrimSpace(stringValue(blocks[0]["type"])) != "text" {
		return normalized
	}
	text, ok := blocks[0]["text"].(string)
	if !ok || strings.TrimSpace(text) == "" {
		return normalized
	}

	// 小文本才做精确 JSON 语义比较。MCP client 已把 structuredContent 解成 JSON 原生值，
	// 可以直接与 text 的解码结果比较，避免再把可能很大的 structuredContent Marshal/Unmarshal 一遍。
	if len(text) <= CallToolMirrorVerifyMaxBytes {
		if callToolTextMirrorsStructured(text, structured) {
			delete(normalized, "content")
		}
		return normalized
	}

	// 对超大单文本不做昂贵的全量 JSON 解析，也不武断认定它一定是 structuredContent 的镜像。
	// 模型侧保留一个很小的预览和省略标记，既不会让文本副本吞掉预算，也不会把合法独立正文完全抹掉。
	normalized["content"] = []any{map[string]any{
		"type": "text",
		"text": callToolLargeTextPreview(text),
	}}
	normalized["_chatdock_content_omitted"] = map[string]any{
		"reason":         "large_text_with_structured_content",
		"original_bytes": len(text),
	}
	return normalized
}

func looksLikeCallToolResult(result map[string]any) bool {
	content, hasContent := result["content"]
	if !hasContent {
		return false
	}
	blocks, ok := callToolContentBlocks(content)
	if !ok {
		return false
	}
	if len(blocks) > 0 {
		return true
	}
	_, hasStructured := result["structuredContent"]
	_, hasIsError := result["isError"]
	return hasStructured || hasIsError
}

func callToolTextMirrorsStructured(text string, structured any) bool {
	var textValue any
	if err := json.Unmarshal([]byte(text), &textValue); err != nil {
		return false
	}
	return reflect.DeepEqual(textValue, structured)
}

func callToolLargeTextPreview(text string) string {
	if len(text) <= callToolLargeTextPreviewBytes {
		return text
	}
	end := callToolLargeTextPreviewBytes
	for end > 0 && !utf8.ValidString(text[:end]) {
		end--
	}
	return text[:end] + fmt.Sprintf("\n...[ChatDock 已省略其余文本，原始 %d 字节]", len(text))
}

func callToolContentBlocks(value any) ([]map[string]any, bool) {
	var blocks []map[string]any
	switch items := value.(type) {
	case []any:
		blocks = make([]map[string]any, 0, len(items))
		for _, item := range items {
			block, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			blocks = append(blocks, block)
		}
	case []map[string]any:
		blocks = items
	default:
		return nil, false
	}
	for _, block := range blocks {
		if strings.TrimSpace(stringValue(block["type"])) == "" {
			return nil, false
		}
	}
	return blocks, true
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
