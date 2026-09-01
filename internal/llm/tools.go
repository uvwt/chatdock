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

	"chatdock/internal/mcp"
	"chatdock/internal/model"
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
	Usage     *model.Usage
}

type MCPToolLoopOptions struct {
	RefreshTools       func() []mcp.MCPTool
	AfterToolRound     func() ([]map[string]any, error)
	OnToolCall         func()
	OnUsage            func(model.Usage)
	ServerInstructions []mcp.MCPServerInstruction
	ContextCheckpoint  *ContextCheckpoint
	OnContextPrepared  func(ContextPreparation) error
}

const finalToolResponseInstruction = "请根据已经完成的工具调用结果给出明确、完整的最终答复。如果工具调用失败，请说明失败原因和下一步。不要继续调用工具，也不要返回空内容。"

func (c *ChatClient) CompleteWithMCPTools(ctx context.Context, cfg model.ModelConfig, history []model.Message, tools []mcp.MCPTool, call func(string, map[string]any) (any, error)) (string, error) {
	return c.CompleteWithMCPToolsEvents(ctx, cfg, history, tools, call, nil)
}

func (c *ChatClient) CompleteWithMCPToolsEvents(ctx context.Context, cfg model.ModelConfig, history []model.Message, tools []mcp.MCPTool, call func(string, map[string]any) (any, error), emit func(string, any) error, options ...MCPToolLoopOptions) (string, error) {
	loopOptions := MCPToolLoopOptions{}
	if len(options) > 0 {
		loopOptions = options[0]
	}
	emitModelEvent := func(event string, value any) error {
		if event == "usage" && loopOptions.OnUsage != nil {
			switch usage := value.(type) {
			case model.Usage:
				loopOptions.OnUsage(usage)
			case *model.Usage:
				if usage != nil {
					loopOptions.OnUsage(*usage)
				}
			}
		}
		if emit == nil {
			return nil
		}
		return emit(event, value)
	}
	prepared, err := PrepareChatContextWithCheckpoint(cfg, history, loopOptions.ContextCheckpoint)
	if err != nil {
		return "", err
	}
	if loopOptions.OnContextPrepared != nil {
		if err := loopOptions.OnContextPrepared(prepared); err != nil {
			return "", err
		}
	}
	messages := buildAnyMessagesFromPreparation(prepared, historyAfterCheckpoint(history, loopOptions.ContextCheckpoint))
	messages = appendMCPContext(messages, tools, loopOptions.ServerInstructions)
	currentTools := func() []map[string]any {
		if loopOptions.RefreshTools != nil {
			return MCPToolsToOpenAITools(loopOptions.RefreshTools())
		}
		return MCPToolsToOpenAITools(tools)
	}
	openAITools := currentTools()
	if len(openAITools) == 0 || call == nil {
		fitted, _, fitErr := FitRawMessagesForContext(cfg, messages, nil)
		if fitErr != nil {
			return "", fitErr
		}
		messages = fitted
		if emit != nil {
			response, err := c.streamChatWithRawMessages(ctx, cfg, messages, nil, emitModelEvent)
			if err != nil && IsContextTooLargeModelError(err) && strings.TrimSpace(response.Content) == "" {
				if fitted, _, aggressiveErr := FitRawMessagesForContextAggressive(cfg, messages, nil); aggressiveErr == nil {
					response, err = c.streamChatWithRawMessages(ctx, cfg, fitted, nil, emitModelEvent)
				} else {
					err = aggressiveErr
				}
			}
			return strings.TrimSpace(response.Content), err
		}
		response, err := c.completeChatWithRawMessages(ctx, cfg, messages, nil)
		if err != nil && IsContextTooLargeModelError(err) && strings.TrimSpace(response.Content) == "" {
			if fitted, _, aggressiveErr := FitRawMessagesForContextAggressive(cfg, messages, nil); aggressiveErr == nil {
				response, err = c.completeChatWithRawMessages(ctx, cfg, fitted, nil)
			} else {
				err = aggressiveErr
			}
		}
		if err != nil {
			return "", err
		}
		if response.Usage != nil && loopOptions.OnUsage != nil {
			loopOptions.OnUsage(*response.Usage)
		}
		answer := strings.TrimSpace(response.Content)
		if cfg.HideThinking {
			answer = StripThinkingContent(answer)
		}
		if answer == "" {
			return "", ErrEmptyModelContent
		}
		return answer, nil
	}

	if emit == nil {
		return c.completeWithMCPToolsBlocking(ctx, cfg, messages, currentTools, call, loopOptions)
	}

	currentToolMessagesStart := len(messages)
	var visibleAnswer strings.Builder
	toolRounds := 0
	finalResponseRequested := false
	emergencyCompressionUsed := false
	for {
		fitted, _, fitErr := FitRawMessagesForContext(cfg, messages, openAITools)
		if fitErr != nil {
			return "", fitErr
		}
		messages = fitted
		resp, err := c.streamChatWithRawMessages(ctx, cfg, messages, openAITools, emitModelEvent)
		if err != nil && IsContextTooLargeModelError(err) && !emergencyCompressionUsed && visibleAnswer.Len() == 0 && strings.TrimSpace(resp.Content) == "" && toolRounds == 0 {
			emergencyCompressionUsed = true
			if fitted, _, aggressiveErr := FitRawMessagesForContextAggressive(cfg, messages, openAITools); aggressiveErr == nil {
				messages = fitted
				resp, err = c.streamChatWithRawMessages(ctx, cfg, messages, openAITools, emitModelEvent)
			} else {
				err = aggressiveErr
			}
		}
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
					openAITools = currentTools()
					continue
				}
			}
			if strings.TrimSpace(resp.Content) == "" {
				if toolRounds == 0 || finalResponseRequested {
					return strings.TrimSpace(visibleAnswer.String()), ErrEmptyModelContent
				}
				// 某些 OpenAI 兼容网关会在工具链结束后返回空的 stop 帧。
				// 再请求一次无工具的最终答复，避免把“无答案”误记为成功。
				messages = append(messages, map[string]any{"role": "user", "content": finalToolResponseInstruction})
				openAITools = nil
				finalResponseRequested = true
				continue
			}
			return strings.TrimSpace(visibleAnswer.String()), nil
		}

		if loopOptions.OnToolCall != nil {
			loopOptions.OnToolCall()
		}
		toolRounds++
		messages = append(messages, assistantToolCallMessage(resp))
		toolMessages, modelMessages, err := executeModelToolCalls(resp.ToolCalls, call, emitModelEvent)
		if err != nil {
			return strings.TrimSpace(visibleAnswer.String()), err
		}
		messages = append(messages, toolMessages...)
		rebalanceToolContent(messages, currentToolMessagesStart, currentToolAggregateMaxBytes)
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

func (c *ChatClient) completeWithMCPToolsBlocking(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, currentTools func() []map[string]any, call func(string, map[string]any) (any, error), options MCPToolLoopOptions) (string, error) {
	currentToolMessagesStart := len(messages)
	toolRounds := 0
	finalResponseRequested := false
	emergencyCompressionUsed := false
	for {
		openAITools := currentTools()
		if finalResponseRequested {
			openAITools = nil
		}
		fitted, _, fitErr := FitRawMessagesForContext(cfg, messages, openAITools)
		if fitErr != nil {
			return "", fitErr
		}
		messages = fitted
		resp, err := c.completeChatWithRawMessages(ctx, cfg, messages, openAITools)
		if err != nil && IsContextTooLargeModelError(err) && !emergencyCompressionUsed && strings.TrimSpace(resp.Content) == "" && toolRounds == 0 {
			emergencyCompressionUsed = true
			if fitted, _, aggressiveErr := FitRawMessagesForContextAggressive(cfg, messages, openAITools); aggressiveErr == nil {
				messages = fitted
				resp, err = c.completeChatWithRawMessages(ctx, cfg, messages, openAITools)
			} else {
				err = aggressiveErr
			}
		}
		if err != nil {
			return "", err
		}
		if resp.Usage != nil && options.OnUsage != nil {
			options.OnUsage(*resp.Usage)
		}
		if len(resp.ToolCalls) == 0 {
			answer := strings.TrimSpace(resp.Content)
			if cfg.HideThinking {
				answer = StripThinkingContent(answer)
			}
			if answer == "" {
				if toolRounds == 0 || finalResponseRequested {
					return "", ErrEmptyModelContent
				}
				messages = append(messages, map[string]any{"role": "user", "content": finalToolResponseInstruction})
				finalResponseRequested = true
				continue
			}
			return answer, nil
		}
		if options.OnToolCall != nil {
			options.OnToolCall()
		}
		toolRounds++
		messages = append(messages, assistantToolCallMessage(resp))
		toolMessages, modelMessages, err := executeModelToolCalls(resp.ToolCalls, call, nil)
		if err != nil {
			return "", err
		}
		messages = append(messages, toolMessages...)
		rebalanceToolContent(messages, currentToolMessagesStart, currentToolAggregateMaxBytes)
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
		if mcpResult, ok := normalizedMCPToolResult(result); ok && mcpResult.IsError {
			payload["ok"] = false
			if _, exists := payload["error"]; !exists {
				payload["error"] = mcpToolResultErrorSummary(mcpResult)
			}
		}
		modelPayload := sanitizeToolPayload(payload)
		eventPayload := cloneToolEventPayload(modelPayload)
		if mcpResult, ok := normalizedMCPToolResult(result); ok {
			if app := mcpResult.AppResource(); app != nil {
				eventPayload["mcp_app"] = app
			}
			if appErr := strings.TrimSpace(mcpResult.AppError()); appErr != "" {
				eventPayload["mcp_app_error"] = appErr
			}
		}
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
			"content":      modelToolContent(modelPayload, currentToolResultMaxBytes, "工具结果过长"),
		})
		modelMessages = append(modelMessages, toolModelMessagesFromPayload(payload)...)
	}
	return toolMessages, modelMessages, nil
}

func normalizedMCPToolResult(value any) (mcp.MCPToolResult, bool) {
	switch result := value.(type) {
	case mcp.MCPToolResult:
		return result, true
	case *mcp.MCPToolResult:
		if result != nil {
			return *result, true
		}
	}
	return mcp.MCPToolResult{}, false
}

func mcpToolResultErrorSummary(result mcp.MCPToolResult) string {
	for _, item := range result.Content {
		content, _ := item.(map[string]any)
		if content["type"] != "text" {
			continue
		}
		if text := strings.TrimSpace(fmt.Sprint(content["text"])); text != "" {
			return text
		}
	}
	return "MCP tool returned isError=true"
}

func cloneToolEventPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		out[key] = value
	}
	return out
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
	return appendMCPContext(messages, tools, nil)
}

// BuildProviderSystemPrompt 复用真实供应商请求的消息构造链路，返回首条 system 消息。
// 工具提示、MCP Server instructions、自动摘要和运行时 system 上下文都会按实际请求顺序合并。
func BuildProviderSystemPrompt(cfg model.ModelConfig, history []model.Message, tools []mcp.MCPTool, instructionSets ...[]mcp.MCPServerInstruction) string {
	var instructions []mcp.MCPServerInstruction
	if len(instructionSets) > 0 {
		instructions = instructionSets[0]
	}
	messages := appendMCPContext(BuildChatMessagesAny(cfg, history), tools, instructions)
	if len(messages) == 0 || messages[0]["role"] != "system" {
		return ""
	}
	prompt, _ := messages[0]["content"].(string)
	return strings.TrimSpace(prompt)
}

func (c *ChatClient) streamChatWithRawMessages(ctx context.Context, cfg model.ModelConfig, messages []map[string]any, tools []map[string]any, emit func(string, any) error) (ModelChatResponse, error) {
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	body := map[string]any{"model": cfg.Model, "messages": messages, "temperature": cfg.Temperature, "stream": true}
	body["stream_options"] = map[string]any{"include_usage": true}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	resp, err := c.openModelStream(ctx, cfg, endpoint, body)
	if err != nil {
		return ModelChatResponse{}, err
	}
	defer resp.Body.Close()
	return readModelToolStream(resp.Body, cfg, emit)
}

func readModelToolStream(body io.Reader, cfg model.ModelConfig, emit func(string, any) error) (ModelChatResponse, error) {
	var full strings.Builder
	filter := NewThinkingFilter(cfg.HideThinking)
	calls := newStreamingToolCallAccumulator()
	var reportedUsage *model.Usage
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
		if delta.Usage != nil {
			if reportedUsage == nil {
				reportedUsage = &model.Usage{Source: delta.Usage.Source}
			}
			reportedUsage.Add(*delta.Usage)
			if err := emit("usage", delta.Usage); err != nil {
				return ModelChatResponse{Content: full.String(), ToolCalls: calls.List(), Usage: reportedUsage}, err
			}
		}
		if delta.Empty() {
			continue
		}
		calls.Apply(delta.ToolCalls)
		if delta.ReasoningContent != "" {
			if err := emit("delta", StreamDelta{ReasoningContent: delta.ReasoningContent}); err != nil {
				return ModelChatResponse{Content: full.String(), ToolCalls: calls.List(), Usage: reportedUsage}, err
			}
		}
		visible := filter.Push(delta.Content)
		if visible == "" {
			continue
		}
		full.WriteString(visible)
		if err := emit("delta", StreamDelta{Content: visible}); err != nil {
			return ModelChatResponse{Content: full.String(), ToolCalls: calls.List(), Usage: reportedUsage}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return ModelChatResponse{Content: full.String(), ToolCalls: calls.List(), Usage: reportedUsage}, err
	}
	visible := filter.Flush()
	if visible != "" {
		full.WriteString(visible)
		if err := emit("delta", StreamDelta{Content: visible}); err != nil {
			return ModelChatResponse{Content: full.String(), ToolCalls: calls.List(), Usage: reportedUsage}, err
		}
	}
	return ModelChatResponse{Content: strings.TrimSpace(full.String()), ToolCalls: calls.List(), Usage: reportedUsage}, nil
}

type toolStreamDelta struct {
	Content          string
	ReasoningContent string
	ToolCalls        []streamingToolCallDelta
	Usage            *model.Usage
}

func (d toolStreamDelta) Empty() bool {
	return d.Content == "" && d.ReasoningContent == "" && len(d.ToolCalls) == 0 && (d.Usage == nil || d.Usage.Empty())
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
	Usage   json.RawMessage `json:"usage"`
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
		return toolStreamDelta{}, modelStreamError("model tool stream failed", chunk.Error)
	}
	usage := normalizeUsage(chunk.Usage)
	if len(chunk.Choices) == 0 {
		return toolStreamDelta{Usage: usage}, nil
	}
	delta := chunk.Choices[0].Delta
	return toolStreamDelta{Content: delta.Content, ReasoningContent: delta.ReasoningContent, ToolCalls: delta.ToolCalls, Usage: usage}, nil
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
	usageRaw, _ := json.Marshal(output["usage"])
	return ModelChatResponse{Content: content, ToolCalls: decodeModelToolCalls(message["tool_calls"]), Usage: normalizeUsage(usageRaw)}, nil
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
		out = append(out, map[string]any{"type": "function", "function": map[string]any{"name": name, "description": desc, "parameters": adaptToolSchemaForModelAPI(tool.InputSchema)}})
	}
	return out
}

func BuildChatMessagesAny(cfg model.ModelConfig, history []model.Message) []map[string]any {
	messages, _, err := BuildChatMessagesAnyChecked(cfg, history)
	if err != nil {
		return nil
	}
	return messages
}

func BuildChatMessagesAnyChecked(cfg model.ModelConfig, history []model.Message) ([]map[string]any, ContextBudget, error) {
	return BuildChatMessagesAnyCheckedWithCheckpoint(cfg, history, nil)
}

func BuildChatMessagesAnyCheckedWithCheckpoint(cfg model.ModelConfig, history []model.Message, checkpoint *ContextCheckpoint) ([]map[string]any, ContextBudget, error) {
	prepared, err := PrepareChatContextWithCheckpoint(cfg, history, checkpoint)
	if err != nil {
		return nil, prepared.Budget, err
	}
	return buildAnyMessagesFromPreparation(prepared, historyAfterCheckpoint(history, checkpoint)), prepared.Budget, nil
}

func buildAnyMessagesFromPreparation(prepared ContextPreparation, history []model.Message) []map[string]any {
	valid := validChatHistory(history)
	_, conversation := splitHistorySystemMessages(valid)
	contextMessages := contextMessagesForPreparation(prepared, conversation, historicalToolMessageIndexSet(conversation))
	messages := make([]map[string]any, 0, len(prepared.Messages)*2)
	for index, item := range contextMessages {
		if item.IncludeToolHistory {
			if historical := historicalAssistantMessages(item, index); len(historical) > 0 {
				messages = append(messages, historical...)
				continue
			}
		}
		if strings.TrimSpace(item.Content) != "" {
			messages = append(messages, map[string]any{"role": item.Role, "content": messageContentForModel(item)})
		}
		if imageContent := imageMessageContentForModel(item); imageContent != nil {
			messages = append(messages, map[string]any{"role": "user", "content": imageContent})
		}
	}
	rebalanceToolContent(messages, 0, historicalToolAggregateMaxBytes)
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
