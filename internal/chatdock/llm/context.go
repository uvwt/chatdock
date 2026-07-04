package llm

import (
	"fmt"
	"strings"

	"chatdock/internal/chatdock/model"
)

type ContextMessage = chatContextMessage

type chatContextMessage struct {
	Role             string
	Content          string
	ModelAttachments []model.AttachmentRecord
}

func BuildChatMessages(cfg model.ModelConfig, history []model.Message) []map[string]string {
	prepared := buildChatContextMessages(cfg, history)
	messages := make([]map[string]string, 0, len(prepared))
	for _, item := range prepared {
		messages = append(messages, map[string]string{"role": item.Role, "content": item.Content})
	}
	return messages
}

func BuildChatContextMessages(cfg model.ModelConfig, history []model.Message) []ContextMessage {
	return buildChatContextMessages(cfg, history)
}

func buildChatContextMessages(cfg model.ModelConfig, history []model.Message) []chatContextMessage {
	cfg = model.NormalizeModelConfig(cfg)
	recentCount, summarizeOld := contextPlan(cfg)
	valid := validChatHistory(history)
	start := len(valid) - recentCount
	if start < 0 {
		start = 0
	}

	messages := make([]chatContextMessage, 0, recentCount+2)
	if systemPrompt := buildSystemPrompt(cfg); strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, chatContextMessage{Role: "system", Content: systemPrompt})
	}

	// 自动上下文不是简单丢弃早期历史：超过最近窗口的内容会被提炼成
	// 一条系统摘要，既节省 token，也避免模型完全忘记当前会话的来龙去脉。
	if summarizeOld && start > 0 {
		if summary := summarizeEarlierContext(valid[:start]); summary != "" {
			messages = append(messages, chatContextMessage{Role: "system", Content: summary})
		}
	}

	for _, item := range valid[start:] {
		messages = append(messages, chatContextMessage{Role: item.Role, Content: item.Content, ModelAttachments: item.ModelAttachments})
	}
	return messages
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
		return cfg.MaxContextMessages, false
	default:
		return 12, true
	}
}

func validChatHistory(history []model.Message) []model.Message {
	valid := make([]model.Message, 0, len(history))
	for _, item := range history {
		if item.Role != "user" && item.Role != "assistant" && item.Role != "system" {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		item.Content = content
		valid = append(valid, item)
	}
	return valid
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
		lines = append(lines, fmt.Sprintf("- %s：%s", contextRoleLabel(item.Role), compactContextText(item.Content, 220)))
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

func buildEnabledSkillsPrompt(skills []model.Skill) string {
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
