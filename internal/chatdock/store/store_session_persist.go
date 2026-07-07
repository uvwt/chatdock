package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) loadSessionsLocked() error {
	sessions, err := loadSessionsFromTablesLocked(s.db, s.activeWorkspace)
	if err != nil {
		return err
	}
	for id, session := range sessions {
		if session != nil && strings.TrimSpace(id) != "" {
			s.sessions[id] = session
		}
	}
	return nil
}

func (s *Store) saveSessionLocked(session *model.Session) error {
	return s.saveSessionForWorkspaceLocked(s.activeWorkspace, session)
}

func (s *Store) saveSessionForWorkspaceLocked(prompt string, session *model.Session) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	if err := s.ensureWorkspaceLocked(prompt); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertSessionTablesTx(tx, prompt, session); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.touchWorkspaceLocked(prompt, time.Now())
}
