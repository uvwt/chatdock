package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

const (
	sessionTitleTimeout     = 40 * time.Second
	sessionTitleMaxAttempts = 2
)

func (a *Server) maybeGenerateSessionTitle(ctx context.Context, sessionID string, cfg model.ModelConfig) (*model.Session, error) {
	session, ok, err := a.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	if !shouldGenerateSessionTitle(session) {
		return session, nil
	}
	title, err := a.generateSessionTitle(ctx, cfg, session)
	if err != nil {
		return session, fmt.Errorf("generate session title: %w", err)
	}
	if title == "" {
		return session, fmt.Errorf("generate session title: %w", llm.ErrEmptyModelContent)
	}
	renamed, err := a.store.RenameSession(sessionID, title)
	if err != nil {
		return session, fmt.Errorf("rename session: %w", err)
	}
	return renamed, nil
}

func (a *Server) generateSessionTitle(ctx context.Context, cfg model.ModelConfig, session *model.Session) (string, error) {
	if len(session.Messages) < 2 {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, sessionTitleTimeout)
	defer cancel()

	titleCfg := cfg
	titleCfg.SystemPrompt = "你只负责给聊天会话生成标题。中文优先，不超过16个汉字或32个英文字符；不要引号、句号、冒号、编号、解释；只输出标题。"
	titleCfg.ContextMode = model.ContextModeCustom
	titleCfg.MaxContextMessages = 4
	titleCfg.Temperature = 0.2
	titleCfg.HideThinking = true

	userText := firstUserTitleContext(session.Messages[0])
	assistantText := compactTitleContext(session.Messages[1].Content, 1200)
	prompt := "根据下面首轮对话生成一个简短会话标题。\n\n用户：" + userText + "\n\n助手：" + assistantText
	var lastErr error
	for attempt := 0; attempt < sessionTitleMaxAttempts; attempt++ {
		raw, err := a.client.Complete(ctx, titleCfg, []model.Message{{Role: "user", Content: prompt}})
		if err != nil {
			lastErr = err
			if !errors.Is(err, llm.ErrEmptyModelContent) {
				return "", err
			}
			continue
		}
		title := cleanGeneratedSessionTitle(raw)
		if title != "" {
			return title, nil
		}
		lastErr = llm.ErrEmptyModelContent
	}
	return "", lastErr
}

func shouldGenerateSessionTitle(session *model.Session) bool {
	if session == nil || len(session.Messages) != 2 {
		return false
	}
	first := session.Messages[0]
	second := session.Messages[1]
	if strings.TrimSpace(first.Role) != "user" || strings.TrimSpace(second.Role) != "assistant" || strings.TrimSpace(second.Content) == "" {
		return false
	}
	current := strings.TrimSpace(session.Title)
	if current == "" || current == "新会话" {
		return true
	}
	source := strings.TrimSpace(first.Content)
	if source == "" && len(first.Attachments) > 0 {
		source = "分析附件：" + first.Attachments[0].Name
	}
	return current == fallbackSessionTitle(source)
}

func fallbackSessionTitle(content string) string {
	content = strings.ReplaceAll(strings.TrimSpace(content), "\n", " ")
	runes := []rune(content)
	if len(runes) > 24 {
		runes = runes[:24]
	}
	if len(runes) == 0 {
		return "新会话"
	}
	return string(runes)
}

func compactTitleContext(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return text
}

func firstUserTitleContext(message model.Message) string {
	text := compactTitleContext(message.Content, 900)
	if text != "" || len(message.Attachments) == 0 {
		return text
	}
	names := make([]string, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		name := strings.TrimSpace(attachment.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "用户发送了附件"
	}
	return "用户发送了附件：" + strings.Join(names, "、")
}

func cleanGeneratedSessionTitle(raw string) string {
	title := strings.TrimSpace(llm.StripThinkingContent(raw))
	for _, prefix := range []string{"标题：", "标题:", "会话标题：", "会话标题:"} {
		title = strings.TrimPrefix(title, prefix)
	}
	title = strings.TrimSpace(title)
	if lines := strings.Split(title, "\n"); len(lines) > 0 {
		title = strings.TrimSpace(lines[0])
	}
	title = strings.Trim(title, " \t\r\n\"'`“”‘’《》【】[]()（）。，、：:；;！!？?")
	runes := []rune(title)
	if len(runes) > 32 {
		title = string(runes[:32])
	}
	return strings.TrimSpace(title)
}
