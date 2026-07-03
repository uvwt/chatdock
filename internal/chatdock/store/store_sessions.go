package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

// 会话方法集中处理 model.Session 的生命周期。
// 消息追加和 SQLite 持久化拆在同包其他文件里，避免一个文件同时承载全部会话细节。

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
