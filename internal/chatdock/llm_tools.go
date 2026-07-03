package chatdock

import (
	"bytes"
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

func (c *ChatClient) CompleteWithMCPTools(ctx context.Context, cfg ModelConfig, history []Message, tools []MCPTool, call func(string, map[string]any) (any, error)) (string, error) {
	return c.CompleteWithMCPToolsEvents(ctx, cfg, history, tools, call, nil)
}

func (c *ChatClient) CompleteWithMCPToolsEvents(ctx context.Context, cfg ModelConfig, history []Message, tools []MCPTool, call func(string, map[string]any) (any, error), emit func(string, any) error) (string, error) {
	messages := BuildChatMessagesAny(cfg, history)
	messages = appendMCPToolUseHint(messages, tools)
	openAITools := MCPToolsToOpenAITools(tools)
	if len(openAITools) == 0 || call == nil {
		if emit != nil {
			return c.Stream(ctx, cfg, history, func(delta StreamDelta) error { return emit("delta", delta) })
		}
		return c.Complete(ctx, cfg, history)
	}
	for {
		resp, err := c.completeChatWithRawMessages(ctx, cfg, messages, openAITools)
		if err != nil {
			return "", err
		}
		if len(resp.ToolCalls) == 0 {
			if emit != nil {
				return c.StreamRawMessages(ctx, cfg, messages, func(delta StreamDelta) error { return emit("delta", delta) })
			}
			answer := strings.TrimSpace(resp.Content)
			if cfg.HideThinking {
				answer = StripThinkingContent(answer)
			}
			return answer, nil
		}
		messages = append(messages, map[string]any{"role": "assistant", "content": resp.Content, "tool" + "_calls": encodeModelToolCalls(resp.ToolCalls)})
		for index, tc := range resp.ToolCalls {
			args := map[string]any{}
			if strings.TrimSpace(tc.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = map[string]any{"_raw_arguments": tc.Function.Arguments, "_parse_error": err.Error()}
				}
			}
			if emit != nil {
				_ = emit("tool_call_start", map[string]any{"tool": tc.Function.Name, "arguments": args})
			}
			result, err := call(tc.Function.Name, args)
			payload := map[string]any{"ok": err == nil, "tool": tc.Function.Name, "result": result}
			if err != nil {
				payload["error"] = err.Error()
			}
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", index)
			}
			if emit != nil {
				_ = emit("tool_call_result", payload)
			}
			messages = append(messages, map[string]any{"role": "tool", "tool" + "_call_id": id, "name": tc.Function.Name, "content": compactJSON(payload)})
		}
		// 不要在第一轮工具结果后直接进入最终流式回答。
		// 复杂任务常常需要 template_match -> template_get -> execute 这种多轮工具链；
		// 提前切成无 tools 的最终回答，会让模型只能说“现在读取完整定义”却无法继续调用工具。
	}
}

func appendMCPToolUseHint(messages []map[string]any, tools []MCPTool) []map[string]any {
	if len(tools) == 0 {
		return messages
	}
	hint := map[string]any{"role": "system", "content": "MCP 工具已通过 tools 字段接入。用户要求查询外部环境、读取记忆/任务、操作文件或明确要求使用 MCP 时，优先调用合适工具，拿到结果后再回答；不要声称没有工具权限。"}
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

func (c *ChatClient) completeChatWithRawMessages(ctx context.Context, cfg ModelConfig, messages []map[string]any, tools []map[string]any) (ModelChatResponse, error) {
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

func MCPToolsToOpenAITools(tools []MCPTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := tool.FullName
		if strings.TrimSpace(name) == "" {
			name = toolFullName(tool.Server, tool.Name)
		}
		desc := strings.TrimSpace(tool.Description)
		if desc == "" {
			desc = tool.Title
		}
		out = append(out, map[string]any{"type": "function", "function": map[string]any{"name": name, "description": desc, "parameters": normalizeJSONSchema(tool.InputSchema)}})
	}
	return out
}

func BuildChatMessagesAny(cfg ModelConfig, history []Message) []map[string]any {
	prepared := buildChatContextMessages(cfg, history)
	messages := make([]map[string]any, 0, len(prepared))
	for _, item := range prepared {
		messages = append(messages, map[string]any{"role": item.Role, "content": messageContentForModel(item)})
	}
	return messages
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
