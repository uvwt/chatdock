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
	for i := range messages {
		out[i] = messages[i]
		out[i].Attachments = append([]model.Attachment(nil), messages[i].Attachments...)
		out[i].ModelAttachments = append([]model.AttachmentRecord(nil), messages[i].ModelAttachments...)
		out[i].Parts = cloneMessageParts(messages[i].Parts)
		out[i].Events = cloneMessageEvents(messages[i].Events)
	}
	return out
}

func cloneMessageParts(parts []model.MessagePart) []model.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]model.MessagePart, len(parts))
	for i := range parts {
		out[i] = parts[i]
		if parts[i].Event != nil {
			event := cloneMessageEvent(*parts[i].Event)
			out[i].Event = &event
		}
	}
	return out
}

func cloneMessageEvents(events []model.MessageEvent) []model.MessageEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]model.MessageEvent, len(events))
	for i := range events {
		out[i] = cloneMessageEvent(events[i])
	}
	return out
}

func cloneMessageEvent(event model.MessageEvent) model.MessageEvent {
	if len(event.Details) == 0 {
		return event
	}
	details := make(map[string]any, len(event.Details))
	for key, value := range event.Details {
		details[key] = value
	}
	event.Details = details
	return event
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
