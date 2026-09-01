package store

import (
	"strings"
	"time"

	"chatdock/internal/model"
)

type preparedChatSession struct {
	session       *model.Session
	attachmentIDs []string
	messageID     string
	history       []model.Message
	changed       bool
}

func (s *Store) prepareUserMessageLocked(sessionID string, content string, attachmentIDs []string) (preparedChatSession, error) {
	session, ok, err := s.sessionLocked(sessionID)
	if err != nil {
		return preparedChatSession{}, err
	}
	if !ok {
		return preparedChatSession{}, model.ErrSessionNotFound
	}
	attachments, err := s.attachmentRecordsByIDsLocked(attachmentIDs)
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

func (s *Store) prepareSessionRegenerationLocked(sessionID string) (preparedChatSession, error) {
	session, ok, err := s.sessionLocked(sessionID)
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
	attachments, err := s.attachmentRecordsByIDsLocked(attachmentIDs)
	if err != nil {
		return preparedChatSession{}, err
	}
	history := cloneMessages(session.Messages)
	history[lastIndex].Content = model.BuildUserContentForModel(last.Content, attachments)
	history[lastIndex].ModelAttachments = attachments
	history[lastIndex].Attachments = nil
	return preparedChatSession{session: session, attachmentIDs: attachmentIDs, history: history}, nil
}

func (s *Store) PrepareChat(input model.ChatRequest) (*model.Session, model.ModelConfig, []model.Message, error) {
	_, session, cfg, history, err := s.prepareChatRequest(input, "", false)
	return session, cfg, history, err
}

func (s *Store) PrepareChatJob(input model.ChatRequest, requestID string) (ChatJob, *model.Session, model.ModelConfig, []model.Message, error) {
	return s.prepareChatRequest(input, requestID, true)
}

func (s *Store) prepareChatRequest(input model.ChatRequest, requestID string, createJob bool) (ChatJob, *model.Session, model.ModelConfig, []model.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.modelConfigLocked()
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	cfg, err = s.resolveChatModelConfigLocked(cfg, input.ProviderID, input.Model)
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	var prepared preparedChatSession
	if input.Regenerate {
		prepared, err = s.prepareSessionRegenerationLocked(input.SessionID)
	} else {
		prepared, err = s.prepareUserMessageLocked(input.SessionID, input.Message, input.AttachmentIDs)
	}
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	snapshotChanged := !prepared.session.SystemPromptFrozen || (prepared.session.ProjectID != "" && !prepared.session.ProjectPromptFrozen)
	if err := s.freezeSessionPromptsLocked(prepared.session); err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	cfg.SystemPrompt = BuildFinalSystemPrompt(prepared.session.SystemPromptSnapshot, prepared.session.ProjectPromptSnapshot)
	modelChanged := prepared.session.ProviderID != cfg.ProviderID || prepared.session.Model != cfg.Model
	prepared.session.ProviderID = cfg.ProviderID
	prepared.session.Model = cfg.Model

	tx, err := s.db.Begin()
	if err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if prepared.changed || modelChanged || snapshotChanged {
		if err := bindAttachmentsTx(tx, prepared.session.ID, prepared.messageID, prepared.attachmentIDs); err != nil {
			return ChatJob{}, nil, model.ModelConfig{}, nil, err
		}
		if err := persistSessionTx(tx, prepared.session); err != nil {
			return ChatJob{}, nil, model.ModelConfig{}, nil, err
		}
	}
	var job ChatJob
	if createJob {
		job = newChatJob(prepared.session.ID, requestID, time.Now())
		if err := insertChatJobWith(tx, job); err != nil {
			return ChatJob{}, nil, model.ModelConfig{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ChatJob{}, nil, model.ModelConfig{}, nil, err
	}
	return job, cloneSession(prepared.session), cfg, prepared.history, nil
}

func (s *Store) AppendAssistantMessage(sessionID string, content string) (*model.Session, error) {
	return s.AppendAssistantMessageWithReasoningAndUsage(sessionID, content, "", nil)
}

func (s *Store) AppendAssistantMessageWithReasoning(sessionID string, content string, reasoning string) (*model.Session, error) {
	return s.AppendAssistantMessageWithReasoningAndUsage(sessionID, content, reasoning, nil)
}

func (s *Store) AppendAssistantMessageWithParts(sessionID string, content string, reasoning string, parts []model.MessagePart, events []model.MessageEvent) (*model.Session, error) {
	return s.AppendAssistantMessageWithPartsAndUsage(sessionID, content, reasoning, parts, events, nil)
}

func (s *Store) AppendAssistantMessageWithReasoningAndUsage(sessionID string, content string, reasoning string, usage *model.Usage) (*model.Session, error) {
	return s.AppendAssistantMessageWithPartsAndUsage(sessionID, content, reasoning, nil, nil, usage)
}

func (s *Store) AppendAssistantMessageWithPartsAndUsage(sessionID string, content string, reasoning string, parts []model.MessagePart, events []model.MessageEvent, usage *model.Usage) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionLocked(sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	now := time.Now()
	session.Messages = append(session.Messages, model.Message{ID: model.NewID(), Role: "assistant", Content: content, Reasoning: strings.TrimSpace(reasoning), Parts: cloneMessageParts(parts), Events: cloneMessageEvents(events), Usage: cloneUsage(usage), CreatedAt: now})
	session.UpdatedAt = now
	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) UpsertAssistantMessageCheckpoint(sessionID string, messageID string, content string, reasoning string, parts []model.MessagePart, events []model.MessageEvent, messageError *model.MessageError) (*model.Session, string, error) {
	return s.UpsertAssistantMessageCheckpointWithUsage(sessionID, messageID, content, reasoning, parts, events, messageError, nil)
}

func (s *Store) UpsertAssistantMessageCheckpointWithUsage(sessionID string, messageID string, content string, reasoning string, parts []model.MessagePart, events []model.MessageEvent, messageError *model.MessageError, usage *model.Usage) (*model.Session, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionLocked(sessionID)
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
	if usage != nil {
		message.Usage = cloneUsage(usage)
	}
	message.Error = nil
	if messageError != nil {
		errorCopy := *messageError
		message.Error = &errorCopy
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	session.Messages[index] = message
	session.UpdatedAt = now
	if err := s.saveSessionLocked(session); err != nil {
		return nil, "", err
	}
	return cloneSession(session), messageID, nil
}
