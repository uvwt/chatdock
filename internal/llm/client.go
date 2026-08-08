package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chatdock/internal/model"
)

var ErrEmptyModelContent = errors.New("empty model response content")

type ChatClient struct {
	httpClient *http.Client
}

func NewChatClient() *ChatClient {
	return &ChatClient{httpClient: &http.Client{Timeout: 120 * time.Second}}
}

func (c *ChatClient) Complete(ctx context.Context, cfg model.ModelConfig, history []model.Message) (string, error) {
	messages := BuildChatMessagesAny(cfg, history)
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

	respBody, err := readModelResponseBody(resp, modelResponseBodyLimit(resp, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", modelAPIError("model api failed", resp, respBody)
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

	content := strings.TrimSpace(output.Choices[0].Message.Content)
	if cfg.HideThinking {
		content = StripThinkingContent(content)
	}
	if content == "" {
		return "", ErrEmptyModelContent
	}
	return content, nil
}

func (c *ChatClient) ListModels(ctx context.Context, cfg model.ModelConfig) ([]string, error) {
	cfg = model.NormalizeModelConfig(cfg)
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := readModelResponseBody(resp, modelResponseBodyLimit(resp, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, modelAPIError("获取模型列表失败", resp, respBody)
	}

	var output struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &output); err != nil {
		return nil, fmt.Errorf("获取模型列表失败：上游 /models 没有返回合法 JSON；%s", summarizeModelProviderBody(resp.Header.Get("Content-Type"), respBody))
	}
	models := make([]string, 0, len(output.Data))
	seen := map[string]bool{}
	for _, item := range output.Data {
		name := strings.TrimSpace(item.ID)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		models = append(models, name)
	}
	return models, nil
}

func summarizeModelProviderBody(contentType string, body []byte) string {
	text := strings.TrimSpace(string(body))
	lowerText := strings.ToLower(text)
	lowerContentType := strings.ToLower(contentType)

	// 第三方中转站常把 /models 拦到网页验证页。这里必须收敛为可读错误，
	// 不能把整段 HTML/JS 透传到手机端，否则用户看到的就是源码瀑布。
	if strings.Contains(lowerContentType, "text/html") || strings.HasPrefix(lowerText, "<!doctype html") || strings.HasPrefix(lowerText, "<html") || strings.Contains(lowerText, "cloudflare") || strings.Contains(lowerText, "challenge-platform") {
		return "上游返回 HTML 页面，可能被 Cloudflare 验证/反爬拦截，或 Base URL 不是 OpenAI 兼容的 /v1 地址"
	}
	if text == "" {
		return "上游响应为空"
	}
	runes := []rune(text)
	if len(runes) > 240 {
		text = string(runes[:240]) + "..."
	}
	return text
}

func (c *ChatClient) TestModelProvider(ctx context.Context, cfg model.ModelConfig) error {
	cfg = model.NormalizeModelConfig(cfg)
	cfg.SystemPrompt = "你是 ChatDock 的模型供应商连通性测试助手。请只回复 OK。"
	cfg.Temperature = 0
	_, err := c.Complete(ctx, cfg, []model.Message{{Role: "user", Content: "请回复 OK，用于测试模型供应商连通性。"}})
	return err
}
