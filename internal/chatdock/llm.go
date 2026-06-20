package chatdock

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type ChatClient struct {
	httpClient *http.Client
}

func NewChatClient() *ChatClient {
	return &ChatClient{httpClient: &http.Client{Timeout: 120 * time.Second}}
}

func applyModelRequestParams(body map[string]any, cfg ModelConfig) {
	// Qwen3 / llama.cpp / LM Studio compatible servers usually read this through
	// the chat template renderer. This is the actual thinking switch; do not fake it
	// by mutating the system prompt.
	body["chat_template_kwargs"] = map[string]any{"enable_thinking": cfg.EnableThinking}
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
	applyModelRequestParams(body, cfg)
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

	content := strings.TrimSpace(output.Choices[0].Message.Content)
	if cfg.HideThinking {
		content = StripThinkingContent(content)
	}
	return content, nil
}

func (c *ChatClient) ListModels(ctx context.Context, cfg ModelConfig) ([]string, error) {
	cfg = NormalizeModelConfig(cfg)
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
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model provider models failed: %s: %s", resp.Status, string(respBody))
	}

	var output struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &output); err != nil {
		return nil, err
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

func (c *ChatClient) TestModelProvider(ctx context.Context, cfg ModelConfig) error {
	_, err := c.ListModels(ctx, cfg)
	return err
}

func (c *ChatClient) Stream(ctx context.Context, cfg ModelConfig, history []Message, onDelta func(StreamDelta) error) (string, error) {
	messages := BuildChatMessages(cfg, history)
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"

	body := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": cfg.Temperature,
		"stream":      true,
	}
	applyModelRequestParams(body, cfg)
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
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return "", fmt.Errorf("model api failed: %s: %s", resp.Status, string(respBody))
	}

	var full strings.Builder
	filter := NewThinkingFilter(cfg.HideThinking)
	scanner := bufio.NewScanner(resp.Body)
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
		if err != nil || delta.Empty() {
			continue
		}

		if delta.ReasoningContent != "" && !cfg.HideThinking {
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
	Choices []struct {
		Delta StreamDelta `json:"delta"`
	} `json:"choices"`
}

func parseStreamDelta(data string) (StreamDelta, error) {
	var chunk streamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return StreamDelta{}, err
	}
	if len(chunk.Choices) == 0 {
		return StreamDelta{}, nil
	}
	return chunk.Choices[0].Delta, nil
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
	systemPrompt := buildSystemPrompt(cfg)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemPrompt})
	}

	for _, item := range history[start:] {
		if item.Role != "user" && item.Role != "assistant" && item.Role != "system" {
			continue
		}
		messages = append(messages, map[string]string{"role": item.Role, "content": item.Content})
	}
	return messages
}

func buildSystemPrompt(cfg ModelConfig) string {
	base := strings.TrimSpace(cfg.SystemPrompt)
	skills := buildEnabledSkillsPrompt(cfg.Skills)
	if base == "" {
		return skills
	}
	if skills == "" {
		return base
	}
	return base + "\n\n" + skills
}

func buildEnabledSkillsPrompt(skills []Skill) string {
	items := make([]string, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		content := strings.TrimSpace(skill.Content)
		if !skill.Enabled || name == "" || content == "" {
			continue
		}
		desc := strings.TrimSpace(skill.Description)
		header := "## " + name
		if desc != "" {
			header += "\n" + desc
		}
		items = append(items, header+"\n"+content)
	}
	if len(items) == 0 {
		return ""
	}
	return "# 已启用技能\n\n以下技能是当前会话必须遵循的补充指令。\n\n" + strings.Join(items, "\n\n")
}

var thinkBlockRegexp = regexp.MustCompile(`(?is)<think>.*?</think>`)

func StripThinkingContent(content string) string {
	content = thinkBlockRegexp.ReplaceAllString(content, "")
	lower := strings.ToLower(content)
	for {
		start := strings.Index(lower, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(lower[start:], "</think>")
		if end < 0 {
			content = content[:start]
			break
		}
		end = start + end + len("</think>")
		content = content[:start] + content[end:]
		lower = strings.ToLower(content)
	}
	return strings.TrimSpace(content)
}

type ThinkingFilter struct {
	enabled bool
	buffer  string
	hidden  bool
}

func NewThinkingFilter(hideThinking bool) *ThinkingFilter {
	return &ThinkingFilter{enabled: hideThinking}
}

func (f *ThinkingFilter) Push(delta string) string {
	if !f.enabled || delta == "" {
		return delta
	}

	f.buffer += delta
	var out strings.Builder

	for len(f.buffer) > 0 {
		lower := strings.ToLower(f.buffer)
		if f.hidden {
			end := strings.Index(lower, "</think>")
			if end < 0 {
				keep := min(len(f.buffer), len("</think>")-1)
				if keep > 0 {
					f.buffer = f.buffer[len(f.buffer)-keep:]
				} else {
					f.buffer = ""
				}
				return out.String()
			}
			f.buffer = f.buffer[end+len("</think>"):]
			f.hidden = false
			continue
		}

		start := strings.Index(lower, "<think>")
		if start < 0 {
			keep := commonPrefixSuffixKeep(f.buffer, "<think>")
			emitLen := len(f.buffer) - keep
			if emitLen > 0 {
				out.WriteString(f.buffer[:emitLen])
				f.buffer = f.buffer[emitLen:]
			}
			return out.String()
		}

		if start > 0 {
			out.WriteString(f.buffer[:start])
		}
		f.buffer = f.buffer[start+len("<think>"):]
		f.hidden = true
	}
	return out.String()
}

func (f *ThinkingFilter) Flush() string {
	if !f.enabled || !f.hidden {
		out := f.buffer
		f.buffer = ""
		return out
	}
	f.buffer = ""
	return ""
}

func commonPrefixSuffixKeep(s string, marker string) int {
	maxKeep := min(len(s), len(marker)-1)
	lowerS := strings.ToLower(s)
	lowerMarker := strings.ToLower(marker)
	for keep := maxKeep; keep > 0; keep-- {
		if strings.HasSuffix(lowerS, lowerMarker[:keep]) {
			return keep
		}
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
