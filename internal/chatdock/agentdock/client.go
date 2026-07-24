package agentdock

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"chatdock/internal/chatdock/model"
)

const (
	contextResponseLimit int64 = 512 << 10
	taskResponseLimit          = 2 * 1024 * 1024
)

type Client struct {
	contextURL string
	token      string
	httpClient *http.Client

	mu           sync.Mutex
	context      string
	contextUntil time.Time
}

func NewClient(contextURL, token string) *Client {
	return &Client{
		contextURL: strings.TrimSpace(contextURL),
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 4 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c != nil && c.contextURL != ""
}

func (c *Client) AppendRuntimeContext(ctx context.Context, history []model.Message) []model.Message {
	if c == nil {
		return history
	}
	text := strings.TrimSpace(c.RuntimeContext(ctx))
	if text == "" {
		return history
	}
	out := append([]model.Message(nil), history...)
	return append(out, model.Message{Role: "system", Content: text, CreatedAt: time.Now()})
}

func (c *Client) RuntimeContext(ctx context.Context) string {
	if !c.Configured() {
		return ""
	}
	now := time.Now()
	c.mu.Lock()
	if c.context != "" && now.Before(c.contextUntil) {
		text := c.context
		c.mu.Unlock()
		return text
	}
	stale := c.context
	c.mu.Unlock()

	fetchCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	text, err := c.fetchContext(fetchCtx)
	if err != nil || strings.TrimSpace(text) == "" {
		return stale
	}
	text = compactContext(text, 12000)
	c.mu.Lock()
	c.context = text
	c.contextUntil = now.Add(5 * time.Minute)
	c.mu.Unlock()
	return text
}

func (c *Client) fetchContext(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.contextURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	c.setAuthorization(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", nil
	}
	var payload struct {
		Context string `json:"context"`
		Summary string `json:"summary"`
	}
	if err := decodeBoundedJSON(resp.Body, contextResponseLimit, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Context) != "" {
		return payload.Context, nil
	}
	return payload.Summary, nil
}

func (c *Client) RequestTaskJSON(ctx context.Context, method, runtimePath string, query url.Values) (map[string]any, int, error) {
	if !c.Configured() {
		return nil, 0, fmt.Errorf("AgentDock 任务接口尚未配置")
	}
	target, err := runtimeURL(c.contextURL, runtimePath, query)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	c.setAuthorization(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := decodeBoundedJSON(resp.Body, taskResponseLimit, &payload); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("解析 AgentDock 任务响应失败: %w", err)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, resp.StatusCode, nil
}

func runtimeURL(contextURL, runtimePath string, query url.Values) (string, error) {
	target, err := url.Parse(strings.TrimSpace(contextURL))
	if err != nil {
		return "", fmt.Errorf("AgentDock Context URL 无效: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
		return "", fmt.Errorf("AgentDock Context URL 必须是完整的 HTTP 地址")
	}
	// Context 与任务 Runtime API 由同一个 AgentDock 服务提供；复用同一地址和 Token，
	// 避免任务面板再维护一套容易漂移的连接配置。
	prefix := strings.TrimSuffix(target.Path, "/")
	if strings.HasSuffix(prefix, "/context") {
		prefix = strings.TrimSuffix(prefix, "/context")
	}
	target.Path = strings.TrimRight(prefix, "/") + runtimePath
	target.RawPath = ""
	target.RawQuery = query.Encode()
	target.Fragment = ""
	return target.String(), nil
}

func (c *Client) setAuthorization(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func compactContext(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}
