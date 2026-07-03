package store

import (
	"strings"

	"chatdock/internal/chatdock/model"
)

func sessionPreview(session *model.Session) (string, string) {
	if session == nil || len(session.Messages) == 0 {
		return "", ""
	}
	msg := session.Messages[len(session.Messages)-1]
	content := strings.Join(strings.Fields(msg.Content), " ")
	runes := []rune(content)
	if len(runes) > 120 {
		content = string(runes[:120]) + "…"
	}
	return strings.TrimSpace(msg.Role), content
}

func cloneSession(session *model.Session) *model.Session {
	if session == nil {
		return nil
	}
	copySession := *session
	copySession.Messages = cloneMessages(session.Messages)
	return &copySession
}

func cloneMessages(messages []model.Message) []model.Message {
	out := make([]model.Message, len(messages))
	copy(out, messages)
	return out
}

func makeTitle(content string) string {
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
