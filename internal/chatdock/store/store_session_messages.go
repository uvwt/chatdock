package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) AppendUserMessage(sessionID string, content string) (*model.Session, model.ModelConfig, []model.Message, error) {
	return s.AppendUserMessageWithAttachments(sessionID, content, nil)
}

func (s *Store) AppendUserMessageWithAttachments(sessionID string, content string, attachmentIDs []string) (*model.Session, model.ModelConfig, []model.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, model.ModelConfig{}, nil, model.ErrSessionNotFound
	}

	attachments, err := s.attachmentRecordsByIDsLocked(attachmentIDs)
	if err != nil {
		return nil, model.ModelConfig{}, nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" && len(attachments) == 0 {
		return nil, model.ModelConfig{}, nil, fmt.Errorf("message is empty")
	}

	titleSource := content
	if titleSource == "" && len(attachments) > 0 {
		titleSource = "分析附件：" + attachments[0].Name
	}
	if session.Title == "新会话" || strings.TrimSpace(session.Title) == "" {
		session.Title = makeTitle(titleSource)
	}

	now := time.Now()
	messageID := model.NewID()
	session.Messages = append(session.Messages, model.Message{ID: messageID, Role: "user", Content: content, Attachments: publicAttachments(attachments), CreatedAt: now})
	session.UpdatedAt = now

	if len(attachments) > 0 {
		ids := uniqueAttachmentIDs(attachmentIDs)
		placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		args := []any{sessionID, messageID, s.activePrompt}
		for _, id := range ids {
			args = append(args, id)
		}
		if _, err := s.db.Exec(`UPDATE attachments SET session_id = ?, message_id = ? WHERE prompt = ? AND id IN (`+placeholders+`)`, args...); err != nil {
			return nil, model.ModelConfig{}, nil, err
		}
	}

	if err := s.saveSessionLocked(session); err != nil {
		return nil, model.ModelConfig{}, nil, err
	}

	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return nil, model.ModelConfig{}, nil, err
	}
	cfg.Skills = skills
	history := cloneMessages(session.Messages)
	if len(history) > 0 {
		history[len(history)-1].Content = model.BuildUserContentForModel(content, attachments)
		history[len(history)-1].ModelAttachments = attachments
		history[len(history)-1].Attachments = nil
	}
	return cloneSession(session), cfg, history, nil
}

func (s *Store) AppendAssistantMessage(sessionID string, content string) (*model.Session, error) {
	return s.AppendAssistantMessageWithReasoning(sessionID, content, "")
}

func (s *Store) AppendAssistantMessageWithReasoning(sessionID string, content string, reasoning string) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, model.ErrSessionNotFound
	}

	now := time.Now()
	session.Messages = append(session.Messages, model.Message{ID: model.NewID(), Role: "assistant", Content: content, Reasoning: strings.TrimSpace(reasoning), CreatedAt: now})
	session.UpdatedAt = now

	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}
