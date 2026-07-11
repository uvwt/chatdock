package store

import (
	"database/sql"
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
	if err := prepareSessionForPersistence(session); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := persistSessionTx(tx, workspaceID, session); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) saveSessionAndBindAttachmentsLocked(workspaceID string, session *model.Session, attachmentIDs []string, messageID string) error {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return err
	}
	if err := prepareSessionForPersistence(session); err != nil {
		return err
	}
	attachmentIDs = uniqueAttachmentIDs(attachmentIDs)
	messageID = strings.TrimSpace(messageID)
	if len(attachmentIDs) > 0 && messageID == "" {
		return fmt.Errorf("message id is empty")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := bindAttachmentsTx(tx, workspaceID, session.ID, messageID, attachmentIDs); err != nil {
		return err
	}
	if err := persistSessionTx(tx, workspaceID, session); err != nil {
		return err
	}
	return tx.Commit()
}

func prepareSessionForPersistence(session *model.Session) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	return nil
}

func persistSessionTx(tx *sql.Tx, workspaceID string, session *model.Session) error {
	if err := upsertSessionTablesTx(tx, workspaceID, session); err != nil {
		return err
	}
	return touchWorkspace(tx, workspaceID, time.Now())
}

func bindAttachmentsTx(tx *sql.Tx, workspaceID string, sessionID string, messageID string, attachmentIDs []string) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(attachmentIDs)), ",")
	args := []any{strings.TrimSpace(sessionID), strings.TrimSpace(messageID), workspaceID}
	for _, id := range attachmentIDs {
		args = append(args, id)
	}
	result, err := tx.Exec(`UPDATE attachments SET session_id = ?, message_id = ? WHERE workspace_id = ? AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(len(attachmentIDs)) {
		return fmt.Errorf("attachment binding count = %d, want %d", affected, len(attachmentIDs))
	}
	return nil
}
