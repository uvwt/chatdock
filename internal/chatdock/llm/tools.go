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

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
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

type MCPToolLoopOptions struct {
	RefreshTools   func() []mcp.MCPTool
	AfterToolRound func() ([]map[string]any, error)
}

func (c *ChatClient) CompleteWithMCPTools(ctx context.Context, cfg model.ModelConfig, history []model.Message, tools []mcp.MCPTool, call func(string, map[string]any) (any, error)) (string, error) {
	return c.CompleteWithMCPToolsEvents(ctx, cfg, history, tools, call, nil)
}

func (c *ChatClient) CompleteWithMCPToolsEvents(ctx context.Context, cfg model.ModelConfig, history []model.Message, tools []mcp.MCPTool, call func(string, map[string]any) (any, error), emit func(string, any) error, options ...MCPToolLoopOptions) (string, error) {
	messages := BuildChatMessagesAny(cfg, history)
	messages = appendMCPToolUseHint(messages, tools)
	loopOptions := MCPToolLoopOptions{}
	if len(options) > 0 {
		loopOptions = options[0]
	}
	currentTools := func() []map[string]any {
		if loopOptions.RefreshTools != nil {
			return MCPToolsToOpenAITools(loopOptions.RefreshTools())
		}
		return MCPToolsToOpenAITools(tools)
	}
	openAITools := currentTools()
	if len(openAITools) == 0 || call == nil {
		if emit != nil {
			return c.Stream(ctx, cfg, history, func(delta StreamDelta) error { return emit("delta", delta) })
		}
		return c.Complete(ctx, cfg, history)
	}

	if emit == nil {
		return c.completeWithMCPToolsBlocking(ctx, cfg, messages, currentTools, call)
	}

	var visibleAnswer strings.Builder
	for {
		resp, err := c.streamChatWithRawMessages(ctx, cfg, messages, openAITools, emit)
		if err != nil {
			return "", err
		}
		visibleAnswer.WriteString(resp.Content)
		if len(resp.ToolCalls) == 0 {
			if loopOptions.AfterToolRound != nil {
				guidanceMessages, err := loopOptions.AfterToolRound()
				if err != nil {
					return strings.TrimSpace(visibleAnswer.String()), err
				}
				if len(guidanceMessages) > 0 {
					if strings.TrimSpace(resp.Content) != "" {
						messages = append(messages, map[string]any{"role": "assistant", "content": resp.Content})
					}
					messages = append(messages, guidanceMessages...)
					continue
				}
			}
			return strings.TrimSpace(visibleAnswer.String()), nil
		}

		messages = append(messages, assistantToolCallMessage(resp))
		toolMessages, modelMessages, err := executeModelToolCalls(resp.ToolCalls, call, emit)
		if err != nil {
			return strings.TrimSpace(visibleAnswer.String()), err
		}
		messages = append(messages, toolMessages...)
		messages = append(messages, modelMessages...)
		if loopOptions.AfterToolRound != nil {
			guidanceMessages, err := loopOptions.AfterToolRound()
			if err != nil {
				return strings.TrimSpace(visibleAnswer.String()), err
			}
			if len(guidanceMessages) > 0 {
				messages = append(messages, guidanceMessages...)
			}
		}
		openAITools = currentTools()
		// 工具结果后仍然继续带 tools 流式请求。这样复杂任务可以多轮调用工具，
		// 普通文本也不再被“非流式工具决策 + 二次流式回答”挡住首字。
	}
}

func (c *ChatClient) completeWithMCPToolsBlocking(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, currentTools func() []map[string]any, call func(string, map[string]any) (any, error)) (string, error) {
	for {
		openAITools := currentTools()
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
		messages = append(messages, assistantToolCallMessage(resp))
		toolMessages, modelMessages, err := executeModelToolCalls(resp.ToolCalls, call, nil)
		if err != nil {
			return "", err
		}
		messages = append(messages, toolMessages...)
		messages = append(messages, modelMessages...)
	}
}

func assistantToolCallMessage(response ModelChatResponse) map[string]any {
	return map[string]any{
		"role":       "assistant",
		"content":    response.Content,
		"tool_calls": encodeModelToolCalls(response.ToolCalls),
	}
}

func executeModelToolCalls(calls []ModelToolCall, call func(string, map[string]any) (any, error), emit func(string, any) error) ([]map[string]any, []map[string]any, error) {
	toolMessages := make([]map[string]any, 0, len(calls))
	modelMessages := make([]map[string]any, 0)
	for index, toolCall := range calls {
		args := decodeToolArguments(toolCall.Function.Arguments)
		if emit != nil {
			if err := emit("tool_call_start", map[string]any{"tool": toolCall.Function.Name, "arguments": args}); err != nil {
				return nil, nil, err
			}
		}

		result, callErr := call(toolCall.Function.Name, args)
		payload := map[string]any{"ok": callErr == nil, "tool": toolCall.Function.Name, "result": result}
		if callErr != nil {
			payload["error"] = callErr.Error()
		}
		eventPayload := sanitizeToolPayload(payload)
		if emit != nil {
			if err := emit("tool_call_result", eventPayload); err != nil {
				return nil, nil, err
			}
		}

		callID := toolCall.ID
		if callID == "" {
			callID = fmt.Sprintf("call_%d", index)
		}
		toolMessages = append(toolMessages, map[string]any{
			"role":         "tool",
			"tool_call_id": callID,
			"name":         toolCall.Function.Name,
			"content":      mcp.CompactJSON(eventPayload),
		})
		modelMessages = append(modelMessages, toolModelMessagesFromPayload(payload)...)
	}
	return toolMessages, modelMessages, nil
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

const (
	toolModelContentKey   = "_chatdock_model_content"
	toolModelRoleKey      = "_chatdock_model_role"
	toolModelContentOKKey = "_chatdock_model_content_ok"
)

func sanitizeToolPayload(payload map[string]any) map[string]any {
	clean, ok := stripToolModelFields(payload).(map[string]any)
	if !ok {
		return payload
	}
	return clean
}

func stripToolModelFields(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isToolModelField(key) {
				continue
			}
			out[key] = stripToolModelFields(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, stripToolModelFields(item))
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, stripToolModelFields(item))
		}
		return out
	default:
		return value
	}
}

func isToolModelField(key string) bool {
	switch key {
	case toolModelContentKey, toolModelRoleKey, toolModelContentOKKey:
		return true
	default:
		return false
	}
}

func toolModelMessagesFromPayload(payload map[string]any) []map[string]any {
	result, _ := payload["result"].(map[string]any)
	if len(result) == 0 {
		return nil
	}
	blocks := normalizeToolModelContent(result[toolModelContentKey])
	if len(blocks) == 0 {
		return nil
	}
	role := strings.ToLower(strings.TrimSpace(fmt.Sprint(result[toolModelRoleKey])))
	if role != "user" {
		role = "user"
	}
	return []map[string]any{{"role": role, "content": blocks}}
}

func normalizeToolModelContent(value any) []map[string]any {
	switch v := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), v...)
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, block)
		}
		return out
	case map[string]any:
		return []map[string]any{v}
	default:
		return nil
	}
}

func appendMCPToolUseHint(messages []map[string]any, tools []mcp.MCPTool) []map[string]any {
	if len(tools) == 0 {
		return messages
	}
	hint := map[string]any{"role": "system", "content": "ChatDock MCP 工具已接入。是否调用工具由你根据用户请求自主判断；直接工具可以立即调用。若存在 chatdock_tools_search，说明还有按需工具：先按目标搜索，命中的真实工具会在下一轮直接出现，再按其 schema 直接调用。不要猜测尚未暴露的参数，也不要声称没有工具权限。"}
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
	return mergeLeadingSystemMessagesAny(out)
}

func (c *ChatClient) streamChatWithRawMessages(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, tools []map[string]any, emit func(string, any) error) (ModelChatResponse, error) {
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	body := map[string]any{"model": cfg.Model, "messages": messages, "temperature": cfg.Temperature, "stream": true}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
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
		respBody, err := readModelResponseBody(resp, modelResponseBodyLimit(resp, 4<<20))
		if err != nil {
			return ModelChatResponse{}, err
		}
		return ModelChatResponse{}, modelAPIError("model api failed", resp, respBody)
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
		if err != nil {
			return ModelChatResponse{Content: full.String(), ToolCalls: calls.List()}, err
		}
		if delta.Empty() {
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
	Error   json.RawMessage `json:"error"`
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
		return toolStreamDelta{}, fmt.Errorf("decode model tool stream chunk: %w", err)
	}
	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return toolStreamDelta{}, fmt.Errorf("model tool stream failed: %s", summarizeModelProviderBody("application/json", chunk.Error))
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
	respBody, err := readModelResponseBody(resp, modelResponseBodyLimit(resp, 16<<20))
	if err != nil {
		return ModelChatResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ModelChatResponse{}, modelAPIError("model api failed", resp, respBody)
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
	return ModelChatResponse{Content: content, ToolCalls: decodeModelToolCalls(message["tool_calls"])}, nil
}

func encodeModelToolCalls(calls []ModelToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		arguments := strings.TrimSpace(call.Function.Arguments)
		// 模型流式输出可能在工具参数尚未闭合时提前结束。执行阶段仍保留原始值并返回
		// 解析错误，但写回 assistant.tool_calls 的历史参数必须是合法 JSON；否则严格校验
		// 请求体的供应商会直接拒绝后续整轮对话。
		if arguments == "" || !json.Valid([]byte(arguments)) {
			arguments = "{}"
		}
		out = append(out, map[string]any{"id": call.ID, "type": firstNonEmptyString(call.Type, "function"), "function": map[string]any{"name": call.Function.Name, "arguments": arguments}})
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
	messages := make([]map[string]any, 0, len(prepared)*2)
	for _, item := range prepared {
		messages = append(messages, map[string]any{"role": item.Role, "content": messageContentForModel(item)})
		if imageContent := imageMessageContentForModel(item); imageContent != nil {
			messages = append(messages, map[string]any{"role": "user", "content": imageContent})
		}
	}
	return mergeLeadingSystemMessagesAny(messages)
}

func mergeLeadingSystemMessagesAny(messages []map[string]any) []map[string]any {
	if len(messages) < 2 || messages[0]["role"] != "system" {
		return messages
	}
	parts := make([]string, 0)
	idx := 0
	for idx < len(messages) && messages[idx]["role"] == "system" {
		if text := strings.TrimSpace(fmt.Sprint(messages[idx]["content"])); text != "" {
			parts = append(parts, text)
		}
		idx++
	}
	if len(parts) <= 1 {
		return messages
	}
	out := make([]map[string]any, 0, len(messages)-len(parts)+1)
	out = append(out, map[string]any{"role": "system", "content": strings.Join(parts, "\n\n---\n\n")})
	out = append(out, messages[idx:]...)
	return out
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
