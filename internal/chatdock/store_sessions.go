package chatdock

import (
	"chatdock/internal/chatdock/model"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 会话方法集中处理 model.Session 的生命周期和消息追加。
// 持久化仍通过 Store 的 SQLite helper 完成，保持单 package 内的简单边界。

func (s *Store) ListSessions() []model.SessionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]model.SessionSummary, 0, len(s.sessions))
	for _, session := range s.sessions {
		lastRole, preview := sessionPreview(session)
		items = append(items, model.SessionSummary{
			ID:        session.ID,
			Title:     session.Title,
			Pinned:    session.Pinned,
			Preview:   preview,
			LastRole:  lastRole,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.UpdatedAt,
			Count:     len(session.Messages),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Pinned != items[j].Pinned {
			return items[i].Pinned
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (s *Store) CreateSession() (*model.Session, error) {
	now := time.Now()
	session := &model.Session{
		ID:        model.NewID(),
		Title:     "新会话",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []model.Message{},
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session
	return cloneSession(session), s.saveSessionLocked(session)
}

func (s *Store) GetSession(id string) (*model.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	return cloneSession(session), true
}

func (s *Store) CloneSession(id string) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, ok := s.sessions[id]
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	now := time.Now()
	copySession := cloneSession(source)
	copySession.ID = model.NewID()
	copySession.Title = strings.TrimSpace(source.Title)
	if copySession.Title == "" {
		copySession.Title = "新会话"
	}
	copySession.Title = strings.TrimSpace(copySession.Title + " 副本")
	if len([]rune(copySession.Title)) > 80 {
		copySession.Title = string([]rune(copySession.Title)[:80])
	}
	copySession.Pinned = false
	copySession.CreatedAt = now
	copySession.UpdatedAt = now
	s.sessions[copySession.ID] = copySession
	if err := s.saveSessionLocked(copySession); err != nil {
		delete(s.sessions, copySession.ID)
		return nil, err
	}
	return cloneSession(copySession), nil
}

func (s *Store) DeleteSession(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return false
	}
	delete(s.sessions, id)
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE prompt = ? AND id = ?`, s.activePrompt, id)
	_ = s.touchPromptLocked(s.activePrompt, time.Now())
	return true
}

func (s *Store) PinSession(id string, pinned bool) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	if session.Pinned == pinned {
		return cloneSession(session), nil
	}
	session.Pinned = pinned
	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) RenameSession(id string, title string) (*model.Session, error) {
	title = strings.TrimSpace(strings.ReplaceAll(title, "\n", " "))
	if title == "" {
		return nil, fmt.Errorf("session title is empty")
	}
	runes := []rune(title)
	if len(runes) > 80 {
		title = string(runes[:80])
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	session.Title = title
	session.UpdatedAt = time.Now()
	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

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

func (s *Store) loadSessionsLocked() error {
	rows, err := s.db.Query(`SELECT json FROM sessions WHERE prompt = ? ORDER BY updated_at DESC`, s.activePrompt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var session model.Session
		if err := json.Unmarshal([]byte(raw), &session); err != nil {
			return err
		}
		if session.ID != "" {
			s.sessions[session.ID] = &session
		}
	}
	return rows.Err()
}

func (s *Store) saveSessionLocked(session *model.Session) error {
	return s.saveSessionForPromptLocked(s.activePrompt, session)
}

func (s *Store) saveSessionForPromptLocked(prompt string, session *model.Session) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	now := time.Now()
	if err := s.ensurePromptLocked(prompt); err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO sessions(prompt, id, json, created_at, updated_at) VALUES(?, ?, ?, ?, ?)
ON CONFLICT(prompt, id) DO UPDATE SET json = excluded.json, updated_at = excluded.updated_at`, prompt, session.ID, string(raw)+"\n", formatDBTime(session.CreatedAt), formatDBTime(session.UpdatedAt))
	if err != nil {
		return err
	}
	return s.touchPromptLocked(prompt, now)
}

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
