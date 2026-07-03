package chatdock

import "chatdock/internal/chatdock/llm"

type ChatClient = llm.ChatClient
type StreamDelta = llm.StreamDelta
type ThinkingFilter = llm.ThinkingFilter
type ModelToolCall = llm.ModelToolCall
type ModelToolCallFunc = llm.ModelToolCallFunc
type ModelChatResponse = llm.ModelChatResponse

func NewChatClient() *ChatClient { return llm.NewChatClient() }

func BuildChatMessages(cfg ModelConfig, history []Message) []map[string]string {
	return llm.BuildChatMessages(cfg, history)
}

func BuildChatMessagesAny(cfg ModelConfig, history []Message) []map[string]any {
	return llm.BuildChatMessagesAny(cfg, history)
}

func MCPToolsToOpenAITools(tools []MCPTool) []map[string]any {
	return llm.MCPToolsToOpenAITools(tools)
}

func StripThinkingContent(content string) string { return llm.StripThinkingContent(content) }

func NewThinkingFilter(hideThinking bool) *ThinkingFilter { return llm.NewThinkingFilter(hideThinking) }

type chatContextMessage = llm.ContextMessage

func firstNonEmptyString(values ...string) string { return llm.FirstNonEmptyString(values...) }

func contextPlan(cfg ModelConfig) (int, bool) { return llm.ContextPlan(cfg) }

func buildChatContextMessages(cfg ModelConfig, history []Message) []chatContextMessage {
	return llm.BuildChatContextMessages(cfg, history)
}

func compactContextText(content string, limit int) string {
	return llm.CompactContextText(content, limit)
}

func buildSystemPrompt(cfg ModelConfig) string { return llm.BuildSystemPrompt(cfg) }

func contextRoleLabel(role string) string { return llm.ContextRoleLabel(role) }
