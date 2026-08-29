package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"chatdock/internal/model"
)

func (c *ChatClient) Stream(ctx context.Context, cfg model.ModelConfig, history []model.Message, onDelta func(StreamDelta) error) (string, error) {
	return c.StreamRawMessages(ctx, cfg, BuildChatMessagesAny(cfg, history), onDelta)
}

func (c *ChatClient) StreamRawMessages(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, onDelta func(StreamDelta) error) (string, error) {
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	body := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": cfg.Temperature,
		"stream":      true,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, err := readModelResponseBody(resp, modelResponseBodyLimit(resp, 4<<20))
		if err != nil {
			return "", err
		}
		return "", modelAPIError("model api failed", resp, respBody)
	}

	return readModelStream(resp.Body, cfg, onDelta)
}

func readModelStream(body io.Reader, cfg model.ModelConfig, onDelta func(StreamDelta) error) (string, error) {
	var full strings.Builder
	filter := NewThinkingFilter(cfg.HideThinking)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		delta, err := parseStreamDelta(data)
		if err != nil {
			return full.String(), err
		}
		if delta.Empty() {
			continue
		}

		if delta.ReasoningContent != "" {
			if err := onDelta(StreamDelta{ReasoningContent: delta.ReasoningContent}); err != nil {
				return full.String(), err
			}
		}

		visible := filter.Push(delta.Content)
		if visible == "" {
			continue
		}
		full.WriteString(visible)
		if err := onDelta(StreamDelta{Content: visible}); err != nil {
			return full.String(), err
		}
	}
	if err := scanner.Err(); err != nil {
		return full.String(), err
	}

	visible := filter.Flush()
	if visible != "" {
		full.WriteString(visible)
		if err := onDelta(StreamDelta{Content: visible}); err != nil {
			return full.String(), err
		}
	}
	return strings.TrimSpace(full.String()), nil
}

type StreamDelta struct {
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

func (d StreamDelta) Empty() bool {
	return d.Content == "" && d.ReasoningContent == ""
}

type streamChunk struct {
	Error   json.RawMessage `json:"error"`
	Choices []struct {
		Delta StreamDelta `json:"delta"`
	} `json:"choices"`
}

func parseStreamDelta(data string) (StreamDelta, error) {
	var chunk streamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return StreamDelta{}, fmt.Errorf("decode model stream chunk: %w", err)
	}
	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return StreamDelta{}, modelStreamError("model stream failed", chunk.Error)
	}
	if len(chunk.Choices) == 0 {
		return StreamDelta{}, nil
	}
	return chunk.Choices[0].Delta, nil
}

// ContextMessage 是模型上下文预览和请求构建共用的内部消息形态。
// 导出这个轻量类型，是为了让 httpapi 层能展示上下文预览，但不接触 LLM 请求细节。
