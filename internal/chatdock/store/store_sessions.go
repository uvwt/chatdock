package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type SessionProjectFilter struct {
	Mode      string
	ProjectID string
}

const (
	SessionProjectFilterAll       = "all"
	SessionProjectFilterByProject = "by_project"
	SessionProjectFilterNoProject = "no_project"
)

func (s *Store) ListSessions(filter SessionProjectFilter) ([]model.SessionSummary, error) {
	items := make([]model.SessionSummary, 0)
	cursor := ""
	for {
		page, nextCursor, hasMore, err := s.ListSessionPage(filter, cursor, 100)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if !hasMore {
			return items, nil
		}
		cursor = nextCursor
	}
}

func (s *Store) CreateSession(projectID string) (*model.Session, error) {
	now := time.Now()
	session := &model.Session{ID: model.NewID(), ProjectID: strings.TrimSpace(projectID), Title: "新会话", CreatedAt: now, UpdatedAt: now, Messages: []model.Message{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	if session.ProjectID != "" {
		exists, err := projectExistsWith(s.db, session.ProjectID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("%w: %s", model.ErrProjectNotFound, session.ProjectID)
		}
	}
	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) GetSession(id string) (*model.Session, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok, err := s.sessionLocked(id)
	if err != nil || !ok {
		return nil, ok, err
	}
	return cloneSession(session), true, nil
}

func (s *Store) UpdateSessionModel(id string, providerID string, modelName string) (*model.Session, error) {
	providerID = strings.TrimSpace(providerID)
	modelName = strings.TrimSpace(modelName)
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionLocked(id)
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
	if err := s.saveSessionLocked(session); err != nil {
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

func (s *Store) CloneSession(id string) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok, err := s.sessionLocked(id)
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
	if err := s.saveSessionLocked(copySession); err != nil {
		return nil, err
	}
	return cloneSession(copySession), nil
}

func (s *Store) BranchSession(id string, messageIndex *int) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok, err := s.sessionLocked(id)
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
	if err := s.saveSessionLocked(branch); err != nil {
		return nil, err
	}
	return cloneSession(branch), nil
}

func (s *Store) EditUserMessageAndTruncate(id string, messageID string, messageIndex *int, content string) (*model.Session, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("message is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionLocked(id)
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
	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) DeleteSession(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok, err := s.sessionLocked(id); err != nil {
		return false, err
	} else if !ok {
		return false, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, strings.TrimSpace(id)); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) PinSession(id string, pinned bool) (*model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok, err := s.sessionLocked(id)
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
	session, ok, err := s.sessionLocked(id)
	if err != nil {
		return nil, err
	}
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
