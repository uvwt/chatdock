package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) sessionForWorkspaceLocked(workspaceID string, sessionID string) (*model.Session, bool, error) {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return nil, false, err
	}
	sessions, err := loadSessionsFromTablesLocked(s.db, workspaceID)
	if err != nil {
		return nil, false, err
	}
	session, ok := sessions[strings.TrimSpace(sessionID)]
	return session, ok, nil
}

func (s *Store) saveSessionForWorkspaceLocked(workspaceID string, session *model.Session) error {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return err
	}
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertSessionTablesTx(tx, workspaceID, session); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.touchWorkspaceLocked(workspaceID, time.Now())
}
