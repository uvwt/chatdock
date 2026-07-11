package store

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type preparedChatSession struct {
	session       *model.Session
	attachmentIDs []string
	messageID     string
	history       []model.Message
	changed       bool
}

func (s *Store) prepareUserMessageLocked(workspaceID string, sessionID string, content string, attachmentIDs []string) (preparedChatSession, error) {
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, sessionID)
	if err != nil {
		return preparedChatSession{}, err
	}
	if !ok {
		return preparedChatSession{}, model.ErrSessionNotFound
	}
	attachments, err := s.attachmentRecordsByIDsLocked(workspaceID, attachmentIDs)
	if err != nil {
		return preparedChatSession{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" && len(attachments) == 0 {
		return preparedChatSession{}, invalidChatRequest("message is empty")
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
	history := cloneMessages(session.Messages)
	history[len(history)-1].Content = model.BuildUserContentForModel(content, attachments)
	history[len(history)-1].ModelAttachments = attachments
	history[len(history)-1].Attachments = nil
	return preparedChatSession{session: session, attachmentIDs: attachmentIDs, messageID: messageID, history: history, changed: true}, nil
}

func (s *Store) prepareSessionRegenerationLocked(workspaceID string, sessionID string) (preparedChatSession, error) {
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, sessionID)
	if err != nil {
		return preparedChatSession{}, err
	}
	if !ok {
		return preparedChatSession{}, model.ErrSessionNotFound
	}
	if len(session.Messages) == 0 {
		return preparedChatSession{}, invalidChatRequest("session has no messages")
	}
	lastIndex := len(session.Messages) - 1
	last := session.Messages[lastIndex]
	if strings.TrimSpace(last.Role) != "user" {
		return preparedChatSession{}, invalidChatRequest("last message is not a user message")
	}
	if strings.TrimSpace(last.Content) == "" && len(last.Attachments) == 0 {
		return preparedChatSession{}, invalidChatRequest("message is empty")
	}
	attachmentIDs := make([]string, 0, len(last.Attachments))
	for _, item := range last.Attachments {
		attachmentIDs = append(attachmentIDs, item.ID)
	}
	attachments, err := s.attachmentRecordsByIDsLocked(workspaceID, attachmentIDs)
	if err != nil {
		return preparedChatSession{}, err
	}
	history := cloneMessages(session.Messages)
	history[lastIndex].Content = model.BuildUserContentForModel(last.Content, attachments)
	history[lastIndex].ModelAttachments = attachments
	history[lastIndex].Attachments = nil
	return preparedChatSession{session: session, attachmentIDs: attachmentIDs, history: history}, nil
}

func (s *Store) PrepareChat(workspaceID string, input model.ChatRequest) (*model.Session, model.ModelConfig, []model.Message, error) {
	_, session, cfg, history, err := s.prepareChatRequest(workspaceID, input, "", false)
	return session, cfg, history, err
}

func (s *Store) PrepareChatJob(workspaceID string, input model.ChatRequest, requestID string) (ChatJob, *model.Session, model.ModelConfig, []model.Message, error) {
	return s.prepareChatRequest(workspaceID, input, requestID, true)
}

func (s *Store) prepareChatRequest(workspaceID string, input model.ChatRequest, requestID string, createJob bool) (ChatJob, *model.Session, model.ModelConfig, []model.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	cfg, err := s.modelConfigForWorkspaceLocked(workspaceID)
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	cfg, err = s.resolveChatModelConfigLocked(cfg, input.ProviderID, input.Model)
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	var prepared preparedChatSession
	if input.Regenerate {
		prepared, err = s.prepareSessionRegenerationLocked(workspaceID, input.SessionID)
	} else {
		prepared, err = s.prepareUserMessageLocked(workspaceID, input.SessionID, input.Message, input.AttachmentIDs)
	}
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	modelChanged := prepared.session.ProviderID != cfg.ProviderID || prepared.session.Model != cfg.Model
	prepared.session.ProviderID = cfg.ProviderID
	prepared.session.Model = cfg.Model

	tx, err := s.db.Begin()
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if prepared.changed || modelChanged {
		if err := bindAttachmentsTx(tx, workspaceID, prepared.session.ID, prepared.messageID, prepared.attachmentIDs); err != nil {
			return ChatJob{}, nil, model.ModelConfig{}, nil, err
		}
		if err := persistSessionTx(tx, workspaceID, prepared.session); err != nil {
			return ChatJob{}, nil, model.ModelConfig{}, nil, err
		}
	}
	var job ChatJob
	if createJob {
		job = newChatJob(workspaceID, prepared.session.ID, requestID, time.Now())
		if err := insertChatJobWith(tx, job); err != nil {
			return ChatJob{}, nil, model.ModelConfig{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	return job, cloneSession(prepared.session), cfg, prepared.history, nil
}

func (s *Store) AppendAssistantMessage(workspaceID string, sessionID string, content string) (*model.Session, error) {
	return s.AppendAssistantMessageWithReasoning(workspaceID, sessionID, content, "")
}

func (s *Store) AppendAssistantMessageWithReasoning(workspaceID string, sessionID string, content string, reasoning string) (*model.Session, error) {
	return s.AppendAssistantMessageWithParts(workspaceID, sessionID, content, reasoning, nil, nil)
}

func (s *Store) AppendAssistantMessageWithParts(workspaceID string, sessionID string, content string, reasoning string, parts []model.MessagePart, events []model.MessageEvent) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	now := time.Now()
	session.Messages = append(session.Messages, model.Message{ID: model.NewID(), Role: "assistant", Content: content, Reasoning: strings.TrimSpace(reasoning), Parts: cloneMessageParts(parts), Events: cloneMessageEvents(events), CreatedAt: now})
	session.UpdatedAt = now
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) UpsertAssistantMessageCheckpoint(workspaceID string, sessionID string, messageID string, content string, reasoning string, parts []model.MessagePart, events []model.MessageEvent) (*model.Session, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, sessionID)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", model.ErrSessionNotFound
	}
	now := time.Now()
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		messageID = model.NewID()
	}
	index := -1
	for i := range session.Messages {
		if session.Messages[i].ID == messageID {
			index = i
			break
		}
	}
	if index < 0 {
		session.Messages = append(session.Messages, model.Message{ID: messageID, Role: "assistant", CreatedAt: now})
		index = len(session.Messages) - 1
	}
	message := session.Messages[index]
	message.ID = messageID
	message.Role = "assistant"
	message.Content = content
	message.Reasoning = strings.TrimSpace(reasoning)
	message.Parts = cloneMessageParts(parts)
	message.Events = cloneMessageEvents(events)
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	session.Messages[index] = message
	session.UpdatedAt = now
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, "", err
	}
	return cloneSession(session), messageID, nil
}
