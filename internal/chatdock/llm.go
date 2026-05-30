package chatdock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatClient struct {
	httpClient *http.Client
}

func NewChatClient() *ChatClient {
	return &ChatClient{httpClient: &http.Client{Timeout: 120 * time.Second}}
}

func (c *ChatClient) Complete(ctx context.Context, cfg ModelConfig, history []Message) (string, error) {
	messages := BuildChatMessages(cfg, history)
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"

	body := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": cfg.Temperature,
		"stream":      false,
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
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("model api failed: %s: %s", resp.Status, string(respBody))
	}

	var output struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &output); err != nil {
		return "", err
	}
	if len(output.Choices) == 0 {
		return "", fmt.Errorf("empty model response")
	}
	return strings.TrimSpace(output.Choices[0].Message.Content), nil
}

func BuildChatMessages(cfg ModelConfig, history []Message) []map[string]string {
	maxMessages := cfg.MaxContextMessages
	if maxMessages <= 0 {
		maxMessages = 12
	}

	start := len(history) - maxMessages
	if start < 0 {
		start = 0
	}

	messages := make([]map[string]string, 0, maxMessages+1)
	if strings.TrimSpace(cfg.SystemPrompt) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": cfg.SystemPrompt})
	}

	for _, item := range history[start:] {
		if item.Role != "user" && item.Role != "assistant" && item.Role != "system" {
			continue
		}
		messages = append(messages, map[string]string{"role": item.Role, "content": item.Content})
	}
	return messages
}
