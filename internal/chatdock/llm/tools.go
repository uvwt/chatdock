package llm

import (
	"bufio"
	"bytes"
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ModelToolCall struct {
	ID       string
	Type     string
	Function ModelToolCallFunc
}

type ModelToolCallFunc struct {
	Name      string
	Arguments string
}

type ModelChatResponse struct {
	Content   string
	ToolCalls []ModelToolCall
}

func (c *ChatClient) CompleteWithMCPTools(ctx context.Context, cfg model.ModelConfig, history []model.Message, tools []mcp.MCPTool, call func(string, map[string]any) (any, error)) (string, error) {
	return c.CompleteWithMCPToolsEvents(ctx, cfg, history, tools, call, nil)
}

func (c *ChatClient) CompleteWithMCPToolsEvents(ctx context.Context, cfg model.ModelConfig, history []model.Message, tools []mcp.MCPTool, call func(string, map[string]any) (any, error), emit func(string, any) error) (string, error) {
	messages := BuildChatMessagesAny(cfg, history)
	messages = appendMCPToolUseHint(messages, tools)
	openAITools := MCPToolsToOpenAITools(tools)
	if len(openAITools) == 0 || call == nil {
		if emit != nil {
			return c.Stream(ctx, cfg, history, func(delta StreamDelta) error { return emit("delta", delta) })
		}
		return c.Complete(ctx, cfg, history)
	}

	if emit == nil {
		return c.completeWithMCPToolsBlocking(ctx, cfg, messages, openAITools, call)
	}

	var visibleAnswer strings.Builder
	for {
		resp, err := c.streamChatWithRawMessages(ctx, cfg, messages, openAITools, emit)
		if err != nil {
			return "", err
		}
		visibleAnswer.WriteString(resp.Content)
		if len(resp.ToolCalls) == 0 {
			return strings.TrimSpace(visibleAnswer.String()), nil
		}

		messages = append(messages, map[string]any{"role": "assistant", "content": resp.Content, "tool" + "_calls": encodeModelToolCalls(resp.ToolCalls)})
		for index, tc := range resp.ToolCalls {
			args := decodeToolArguments(tc.Function.Arguments)
			_ = emit("tool_call_start", map[string]any{"tool": tc.Function.Name, "arguments": args})
			result, err := call(tc.Function.Name, args)
			payload := map[string]any{"ok": err == nil, "tool": tc.Function.Name, "result": result}
			if err != nil {
				payload["error"] = err.Error()
			}
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", index)
			}
			_ = emit("tool_call_result", payload)
			messages = append(messages, map[string]any{"role": "tool", "tool" + "_call_id": id, "name": tc.Function.Name, "content": mcp.CompactJSON(payload)})
		}
		// 工具结果后仍然继续带 tools 流式请求。这样复杂任务可以多轮调用工具，
		// 普通文本也不再被“非流式工具决策 + 二次流式回答”挡住首字。
	}
}

func (c *ChatClient) completeWithMCPToolsBlocking(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, openAITools []map[string]any, call func(string, map[string]any) (any, error)) (string, error) {
	for {
		resp, err := c.completeChatWithRawMessages(ctx, cfg, messages, openAITools)
		if err != nil {
			return "", err
		}
		if len(resp.ToolCalls) == 0 {
			answer := strings.TrimSpace(resp.Content)
			if cfg.HideThinking {
				answer = StripThinkingContent(answer)
			}
			return answer, nil
		}
		messages = append(messages, map[string]any{"role": "assistant", "content": resp.Content, "tool" + "_calls": encodeModelToolCalls(resp.ToolCalls)})
		for index, tc := range resp.ToolCalls {
			args := decodeToolArguments(tc.Function.Arguments)
			result, err := call(tc.Function.Name, args)
			payload := map[string]any{"ok": err == nil, "tool": tc.Function.Name, "result": result}
			if err != nil {
				payload["error"] = err.Error()
			}
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", index)
			}
			messages = append(messages, map[string]any{"role": "tool", "tool" + "_call_id": id, "name": tc.Function.Name, "content": mcp.CompactJSON(payload)})
		}
	}
}

func decodeToolArguments(raw string) map[string]any {
	args := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return args
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return map[string]any{"_raw_arguments": raw, "_parse_error": err.Error()}
	}
	return args
}

func appendMCPToolUseHint(messages []map[string]any, tools []mcp.MCPTool) []map[string]any {
	if len(tools) == 0 {
		return messages
	}
	hint := map[string]any{"role": "system", "content": "ChatDock 内置工具和 MCP 工具已通过 tools 字段接入。用户要求管理定时任务、查询外部环境、读取记忆/任务、操作文件或明确要求使用工具时，优先调用合适工具，拿到结果后再回答；不要声称没有工具权限。"}
	out := make([]map[string]any, 0, len(messages)+1)
	inserted := false
	for _, msg := range messages {
		if !inserted && msg["role"] != "system" {
			out = append(out, hint)
			inserted = true
		}
		out = append(out, msg)
	}
	if !inserted {
		out = append(out, hint)
	}
	return out
}

func (c *ChatClient) streamChatWithRawMessages(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, tools []map[string]any, emit func(string, any) error) (ModelChatResponse, error) {
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	body := map[string]any{"model": cfg.Model, "messages": messages, "temperature": cfg.Temperature, "stream": true}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	applyModelRequestParams(body, cfg)
	raw, err := json.Marshal(body)
	if err != nil {
		return ModelChatResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ModelChatResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ModelChatResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return ModelChatResponse{}, fmt.Errorf("model api failed: %s: %s", resp.Status, string(respBody))
	}
	return readModelToolStream(resp.Body, cfg, emit)
}

func readModelToolStream(body io.Reader, cfg model.ModelConfig, emit func(string, any) error) (ModelChatResponse, error) {
	var full strings.Builder
	filter := NewThinkingFilter(cfg.HideThinking)
	calls := newStreamingToolCallAccumulator()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		delta, err := parseToolStreamDelta(data)
		if err != nil || delta.Empty() {
			continue
		}
		calls.Apply(delta.ToolCalls)
		if delta.ReasoningContent != "" {
			if err := emit("delta", StreamDelta{ReasoningContent: delta.ReasoningContent}); err != nil {
				return ModelChatResponse{Content: full.String(), ToolCalls: calls.List()}, err
			}
		}
		visible := filter.Push(delta.Content)
		if visible == "" {
			continue
		}
		full.WriteString(visible)
		if err := emit("delta", StreamDelta{Content: visible}); err != nil {
			return ModelChatResponse{Content: full.String(), ToolCalls: calls.List()}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return ModelChatResponse{Content: full.String(), ToolCalls: calls.List()}, err
	}
	visible := filter.Flush()
	if visible != "" {
		full.WriteString(visible)
		if err := emit("delta", StreamDelta{Content: visible}); err != nil {
			return ModelChatResponse{Content: full.String(), ToolCalls: calls.List()}, err
		}
	}
	return ModelChatResponse{Content: strings.TrimSpace(full.String()), ToolCalls: calls.List()}, nil
}

type toolStreamDelta struct {
	Content          string
	ReasoningContent string
	ToolCalls        []streamingToolCallDelta
}

func (d toolStreamDelta) Empty() bool {
	return d.Content == "" && d.ReasoningContent == "" && len(d.ToolCalls) == 0
}

type streamingToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string                   `json:"content"`
			ReasoningContent string                   `json:"reasoning_content"`
			ToolCalls        []streamingToolCallDelta `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

func parseToolStreamDelta(data string) (toolStreamDelta, error) {
	var chunk toolStreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return toolStreamDelta{}, err
	}
	if len(chunk.Choices) == 0 {
		return toolStreamDelta{}, nil
	}
	delta := chunk.Choices[0].Delta
	return toolStreamDelta{Content: delta.Content, ReasoningContent: delta.ReasoningContent, ToolCalls: delta.ToolCalls}, nil
}

type streamingToolCallAccumulator struct {
	order []int
	calls map[int]*ModelToolCall
}

func newStreamingToolCallAccumulator() *streamingToolCallAccumulator {
	return &streamingToolCallAccumulator{calls: map[int]*ModelToolCall{}}
}

func (a *streamingToolCallAccumulator) Apply(deltas []streamingToolCallDelta) {
	for _, delta := range deltas {
		call, ok := a.calls[delta.Index]
		if !ok {
			a.order = append(a.order, delta.Index)
			call = &ModelToolCall{Type: "function"}
			a.calls[delta.Index] = call
		}
		if delta.ID != "" {
			call.ID = delta.ID
		}
		if delta.Type != "" {
			call.Type = delta.Type
		}
		call.Function.Name += delta.Function.Name
		call.Function.Arguments += delta.Function.Arguments
	}
}

func (a *streamingToolCallAccumulator) List() []ModelToolCall {
	out := make([]ModelToolCall, 0, len(a.order))
	for _, index := range a.order {
		call := a.calls[index]
		if call == nil || strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", index)
		}
		if call.Type == "" {
			call.Type = "function"
		}
		out = append(out, *call)
	}
	return out
}

func (c *ChatClient) completeChatWithRawMessages(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, tools []map[string]any) (ModelChatResponse, error) {
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	body := map[string]any{"model": cfg.Model, "messages": messages, "temperature": cfg.Temperature, "stream": false}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	applyModelRequestParams(body, cfg)
	raw, err := json.Marshal(body)
	if err != nil {
		return ModelChatResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ModelChatResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ModelChatResponse{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ModelChatResponse{}, fmt.Errorf("model api failed: %s: %s", resp.Status, string(respBody))
	}
	var output map[string]any
	if err := json.Unmarshal(respBody, &output); err != nil {
		return ModelChatResponse{}, err
	}
	choices, _ := output["choices"].([]any)
	if len(choices) == 0 {
		return ModelChatResponse{}, fmt.Errorf("empty model response")
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	content, _ := message["content"].(string)
	return ModelChatResponse{Content: content, ToolCalls: decodeModelToolCalls(message["tool"+"_calls"])}, nil
}

func encodeModelToolCalls(calls []ModelToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		out = append(out, map[string]any{"id": call.ID, "type": firstNonEmptyString(call.Type, "function"), "function": map[string]any{"name": call.Function.Name, "arguments": call.Function.Arguments}})
	}
	return out
}

func decodeModelToolCalls(value any) []ModelToolCall {
	items, _ := value.([]any)
	out := make([]ModelToolCall, 0, len(items))
	for i, item := range items {
		m, _ := item.(map[string]any)
		fn, _ := m["function"].(map[string]any)
		id, _ := m["id"].(string)
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		typ, _ := m["type"].(string)
		name, _ := fn["name"].(string)
		arguments, _ := fn["arguments"].(string)
		if name == "" {
			continue
		}
		if typ == "" {
			typ = "function"
		}
		out = append(out, ModelToolCall{ID: id, Type: typ, Function: ModelToolCallFunc{Name: name, Arguments: arguments}})
	}
	return out
}

func MCPToolsToOpenAITools(tools []mcp.MCPTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := tool.FullName
		if strings.TrimSpace(name) == "" {
			name = mcp.ToolFullName(tool.Server, tool.Name)
		}
		desc := strings.TrimSpace(tool.Description)
		if desc == "" {
			desc = tool.Title
		}
		out = append(out, map[string]any{"type": "function", "function": map[string]any{"name": name, "description": desc, "parameters": mcp.NormalizeJSONSchema(tool.InputSchema)}})
	}
	return out
}

func BuildChatMessagesAny(cfg model.ModelConfig, history []model.Message) []map[string]any {
	prepared := buildChatContextMessages(cfg, history)
	messages := make([]map[string]any, 0, len(prepared))
	for _, item := range prepared {
		messages = append(messages, map[string]any{"role": item.Role, "content": messageContentForModel(item)})
	}
	return messages
}

func FirstNonEmptyString(values ...string) string {
	return firstNonEmptyString(values...)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
