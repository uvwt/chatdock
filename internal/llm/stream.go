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
	messages, _, err := BuildChatMessagesAnyChecked(cfg, history)
	if err != nil {
		return "", err
	}
	return c.StreamRawMessages(ctx, cfg, messages, onDelta)
}

func (c *ChatClient) StreamRawMessages(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, onDelta func(StreamDelta) error) (string, error) {
	fitted, _, err := FitRawMessagesForContext(cfg, messages, nil)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	body := map[string]any{
		"model":       cfg.Model,
		"messages":    fitted,
		"temperature": cfg.Temperature,
		"stream":      true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}
	resp, err := c.openModelStream(ctx, cfg, endpoint, body)
	if err != nil {
		if !IsContextTooLargeModelError(err) {
			return "", err
		}
		aggressive, _, aggressiveErr := FitRawMessagesForContextAggressive(cfg, fitted, nil)
		if aggressiveErr != nil {
			return "", aggressiveErr
		}
		body["messages"] = aggressive
		resp, err = c.openModelStream(ctx, cfg, endpoint, body)
		if err != nil {
			return "", err
		}
	}
	defer resp.Body.Close()

	answer, err := readModelStream(resp.Body, cfg, onDelta)
	if err != nil && IsContextTooLargeModelError(err) && strings.TrimSpace(answer) == "" {
		aggressive, _, aggressiveErr := FitRawMessagesForContextAggressive(cfg, fitted, nil)
		if aggressiveErr != nil {
			return "", aggressiveErr
		}
		body["messages"] = aggressive
		resp, err = c.openModelStream(ctx, cfg, endpoint, body)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		return readModelStream(resp.Body, cfg, onDelta)
	}
	return answer, err
}

func (c *ChatClient) openModelStream(ctx context.Context, cfg model.ModelConfig, endpoint string, body map[string]any) (*http.Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		requestBody := cloneMap(body)
		if attempt > 0 {
			delete(requestBody, "stream_options")
		}
		raw, err := json.Marshal(requestBody)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		respBody, readErr := readModelResponseBody(resp, modelResponseBodyLimit(resp, 4<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if attempt == 0 && streamOptionsUnsupported(respBody) {
			continue
		}
		return nil, modelAPIError("model api failed", resp, respBody)
	}
	return nil, fmt.Errorf("model stream request failed")
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func streamOptionsUnsupported(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "stream_options") || strings.Contains(text, "stream options") ||
		(strings.Contains(text, "include_usage") && (strings.Contains(text, "unknown") || strings.Contains(text, "unsupported") || strings.Contains(text, "invalid")))
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
		if delta.Usage != nil {
			if err := onDelta(StreamDelta{Usage: delta.Usage}); err != nil {
				return full.String(), err
			}
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
	Content          string       `json:"content,omitempty"`
	ReasoningContent string       `json:"reasoning_content,omitempty"`
	Usage            *model.Usage `json:"usage,omitempty"`
}

func (d StreamDelta) Empty() bool {
	return d.Content == "" && d.ReasoningContent == "" && (d.Usage == nil || d.Usage.Empty())
}

type streamChunk struct {
	Error   json.RawMessage `json:"error"`
	Usage   json.RawMessage `json:"usage"`
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
	usage := normalizeUsage(chunk.Usage)
	if len(chunk.Choices) == 0 {
		return StreamDelta{Usage: usage}, nil
	}
	delta := chunk.Choices[0].Delta
	delta.Usage = usage
	return delta, nil
}

// ContextMessage 是模型上下文预览和请求构建共用的内部消息形态。
// 导出这个轻量类型，是为了让 httpapi 层能展示上下文预览，但不接触 LLM 请求细节。
