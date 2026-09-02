package llm

import (
	"fmt"
	"strings"

	"chatdock/internal/model"
)

type ContextMessage = chatContextMessage

type chatContextMessage struct {
	Role               string
	Content            string
	SourceMessageID    string
	SourceMessageIndex int
	ModelAttachments   []model.AttachmentRecord
	Events             []model.MessageEvent
	IncludeToolHistory bool
}

func BuildChatMessages(cfg model.ModelConfig, history []model.Message) []map[string]string {
	prepared := buildChatContextMessages(cfg, history)
	messages := make([]map[string]string, 0, len(prepared))
	for _, item := range prepared {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		messages = append(messages, map[string]string{"role": item.Role, "content": item.Content})
	}
	return mergeLeadingSystemMessages(messages)
}

func mergeLeadingSystemMessages(messages []map[string]string) []map[string]string {
	if len(messages) < 2 || messages[0]["role"] != "system" {
		return messages
	}
	parts := make([]string, 0)
	idx := 0
	for idx < len(messages) && messages[idx]["role"] == "system" {
		if text := strings.TrimSpace(messages[idx]["content"]); text != "" {
			parts = append(parts, text)
		}
		idx++
	}
	if len(parts) <= 1 {
		return messages
	}
	out := make([]map[string]string, 0, len(messages)-len(parts)+1)
	out = append(out, map[string]string{"role": "system", "content": strings.Join(parts, "\n\n---\n\n")})
	out = append(out, messages[idx:]...)
	return out
}

func BuildChatContextMessages(cfg model.ModelConfig, history []model.Message) []ContextMessage {
	return buildChatContextMessages(cfg, history)
}

func buildChatContextMessages(cfg model.ModelConfig, history []model.Message) []chatContextMessage {
	prepared, err := PrepareChatContext(cfg, history)
	if err != nil {
		// 旧的无 error API 仍供预览和兼容调用方使用；真正发送路径会调用
		// checked 版本并把硬上限错误直接交给用户。
		return nil
	}
	return contextMessagesForPreparation(prepared)
}

func contextMessagesForPreparation(prepared ContextPreparation) []chatContextMessage {
	return append([]chatContextMessage(nil), prepared.Messages...)
}

func messageContentForModel(item chatContextMessage) any {
	return item.Content
}

func imageMessageContentForModel(item chatContextMessage) any {
	images := model.ImageContentBlocks(item.ModelAttachments)
	if item.Role != "user" || len(images) == 0 {
		return nil
	}
	blocks := make([]map[string]any, 0, len(images)+1)
	text := strings.TrimSpace(item.Content)
	if text == "" {
		text = "Please analyze the uploaded image."
	}
	blocks = append(blocks, map[string]any{"type": "text", "text": "ChatDock loaded the uploaded image and sends it below as visual input.\n\n" + text})
	blocks = append(blocks, images...)
	return blocks
}

func normalizeContextMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case model.ContextModeCompact:
		return model.ContextModeCompact
	case model.ContextModeExpanded:
		return model.ContextModeExpanded
	case model.ContextModeCustom:
		return model.ContextModeCustom
	default:
		return model.ContextModeAuto
	}
}

func ContextPlan(cfg model.ModelConfig) (int, bool) {
	return contextPlan(cfg)
}

func contextPlan(cfg model.ModelConfig) (int, bool) {
	switch normalizeContextMode(cfg.ContextMode) {
	case model.ContextModeCompact:
		return 8, true
	case model.ContextModeExpanded:
		return 20, true
	case model.ContextModeCustom:
		if cfg.MaxContextMessages <= 0 {
			return 12, false
		}
		if cfg.MaxContextMessages > model.MaxContextMessagesLimit {
			return model.MaxContextMessagesLimit, false
		}
		return cfg.MaxContextMessages, false
	default:
		return 12, true
	}
}

func splitHistorySystemMessages(history []model.Message) ([]model.Message, []model.Message) {
	systems := make([]model.Message, 0)
	conversation := make([]model.Message, 0, len(history))
	for _, item := range history {
		if item.Role == "system" {
			systems = append(systems, item)
			continue
		}
		conversation = append(conversation, item)
	}
	return systems, conversation
}

func validChatHistory(history []model.Message) []model.Message {
	valid := make([]model.Message, 0, len(history))
	for _, item := range history {
		if item.Role != "user" && item.Role != "assistant" && item.Role != "system" {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" && !hasModelImageAttachment(item.ModelAttachments) {
			continue
		}
		item.Content = content
		valid = append(valid, item)
	}
	return valid
}

func hasModelImageAttachment(attachments []model.AttachmentRecord) bool {
	for _, attachment := range attachments {
		if model.IsImageAttachment(attachment) && strings.TrimSpace(attachment.ModelURL) != "" {
			return true
		}
	}
	return false
}

func summarizeEarlierContext(history []model.Message) string {
	if len(history) == 0 {
		return ""
	}
	const maxItems = 18
	start := len(history) - maxItems
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(history[start:])+2)
	if start > 0 {
		lines = append(lines, fmt.Sprintf("- 更早还有 %d 条消息，已省略为摘要前情。", start))
	}
	for _, item := range history[start:] {
		content := item.Content
		if strings.TrimSpace(content) == "" && hasModelImageAttachment(item.ModelAttachments) {
			content = "[图片附件]"
		}
		lines = append(lines, fmt.Sprintf("- %s：%s", contextRoleLabel(item.Role), compactContextText(content, 220)))
	}
	if len(lines) == 0 {
		return ""
	}
	return "# 早期会话摘要\n\n以下内容由 ChatDock 自动从更早的会话历史提炼，用于延续上下文；最近消息仍保留原文。\n" + strings.Join(lines, "\n")
}

func ContextRoleLabel(role string) string {
	return contextRoleLabel(role)
}

func contextRoleLabel(role string) string {
	switch role {
	case "user":
		return "用户"
	case "assistant":
		return "助手"
	case "system":
		return "系统"
	default:
		return role
	}
}

func CompactContextText(content string, limit int) string {
	return compactContextText(content, limit)
}

func compactContextText(content string, limit int) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "..."
}

func BuildSystemPrompt(cfg model.ModelConfig) string {
	return buildSystemPrompt(cfg)
}

func buildSystemPrompt(cfg model.ModelConfig) string {
	return strings.TrimSpace(cfg.SystemPrompt)
}
