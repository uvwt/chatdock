package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

// 会话生命周期全部以显式 workspaceID 为边界；不要把会话放入 Store 全局缓存。

func (s *Store) ListSessions(workspaceID string) ([]model.SessionSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	sessions, err := loadSessionsFromTablesLocked(s.db, workspaceID)
	if err != nil {
		return nil, err
	}
	items := make([]model.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		lastRole, preview := sessionPreview(session)
		items = append(items, model.SessionSummary{ID: session.ID, Title: session.Title, Pinned: session.Pinned, ProviderID: session.ProviderID, Model: session.Model, Preview: preview, LastRole: lastRole, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt, Count: len(session.Messages)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Pinned != items[j].Pinned {
			return items[i].Pinned
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *Store) CreateSession(workspaceID string) (*model.Session, error) {
	now := time.Now()
	session := &model.Session{ID: model.NewID(), Title: "新会话", CreatedAt: now, UpdatedAt: now, Messages: []model.Message{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) GetSession(workspaceID string, id string) (*model.Session, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, id)
	if err != nil || !ok {
		return nil, ok, err
	}
	return cloneSession(session), true, nil
}

func (s *Store) UpdateSessionModel(workspaceID string, id string, providerID string, modelName string) (*model.Session, error) {
	providerID = strings.TrimSpace(providerID)
	modelName = strings.TrimSpace(modelName)
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	if session.ProviderID == providerID && session.Model == modelName {
		return cloneSession(session), nil
	}
	session.ProviderID = providerID
	session.Model = modelName
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func branchTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新会话"
	}
	title = strings.TrimSpace(title + " 分支")
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}
	return title
}

func (s *Store) CloneSession(workspaceID string, id string) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok, err := s.sessionForWorkspaceLocked(workspaceID, id)
	if err != nil {
		return nil, err
	}
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
	if err := s.saveSessionForWorkspaceLocked(workspaceID, copySession); err != nil {
		return nil, err
	}
	return cloneSession(copySession), nil
}

func (s *Store) BranchSession(workspaceID string, id string, messageIndex *int) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok, err := s.sessionForWorkspaceLocked(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	cut := len(source.Messages)
	if messageIndex != nil {
		if *messageIndex < 0 || *messageIndex >= len(source.Messages) {
			return nil, fmt.Errorf("message index out of range")
		}
		cut = *messageIndex + 1
	}
	now := time.Now()
	branch := cloneSession(source)
	branch.ID = model.NewID()
	branch.Title = branchTitle(source.Title)
	branch.Pinned = false
	branch.CreatedAt = now
	branch.UpdatedAt = now
	branch.Messages = cloneMessages(source.Messages[:cut])
	if err := s.saveSessionForWorkspaceLocked(workspaceID, branch); err != nil {
		return nil, err
	}
	return cloneSession(branch), nil
}

func (s *Store) EditUserMessageAndTruncate(workspaceID string, id string, messageID string, messageIndex *int, content string) (*model.Session, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("message is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	index := -1
	messageID = strings.TrimSpace(messageID)
	if messageID != "" {
		for i := range session.Messages {
			if session.Messages[i].ID == messageID {
				index = i
				break
			}
		}
	}
	if index < 0 && messageIndex != nil {
		index = *messageIndex
	}
	if index < 0 || index >= len(session.Messages) {
		return nil, fmt.Errorf("message index out of range")
	}
	if strings.TrimSpace(session.Messages[index].Role) != "user" {
		return nil, fmt.Errorf("only user messages can be edited")
	}
	session.Messages[index].Content = content
	session.Messages = cloneMessages(session.Messages[:index+1])
	session.UpdatedAt = time.Now()
	if index == 0 {
		session.Title = makeTitle(content)
	}
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) DeleteSession(workspaceID string, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return false, err
	}
	if _, ok, err := s.sessionForWorkspaceLocked(workspaceID, id); err != nil {
		return false, err
	} else if !ok {
		return false, nil
	}
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE workspace_id = ? AND id = ?`, workspaceID, strings.TrimSpace(id)); err != nil {
		return false, err
	}
	_ = s.touchWorkspaceLocked(workspaceID, time.Now())
	return true, nil
}

func (s *Store) PinSession(workspaceID string, id string, pinned bool) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	if session.Pinned == pinned {
		return cloneSession(session), nil
	}
	session.Pinned = pinned
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) RenameSession(workspaceID string, id string, title string) (*model.Session, error) {
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
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	session.Title = title
	session.UpdatedAt = time.Now()
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) SessionWorkspace(id string) (string, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var workspaceID string
	err := s.db.QueryRow(`SELECT workspace_id FROM sessions WHERE id = ? ORDER BY updated_at DESC LIMIT 1`, id).Scan(&workspaceID)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "", false, nil
		}
		return "", false, err
	}
	return workspaceID, true, nil
}
