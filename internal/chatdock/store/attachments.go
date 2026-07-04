package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) SaveAttachment(record model.AttachmentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(record.ID) == "" {
		record.ID = model.NewID()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	if strings.TrimSpace(record.Prompt) == "" {
		record.Prompt = s.activePrompt
	}
	_, err := s.db.Exec(`INSERT INTO attachments(prompt, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Prompt, record.ID, record.SessionID, record.MessageID, record.Name, record.MIMEType, record.Size, record.StoragePath, record.SHA256, record.TextContent, record.Status, formatDBTime(record.CreatedAt))
	return err
}

func (s *Store) attachmentRecordsByIDsLocked(ids []string) ([]model.AttachmentRecord, error) {
	ids = uniqueAttachmentIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, s.activePrompt)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(`SELECT prompt, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at FROM attachments WHERE prompt = ? AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]model.AttachmentRecord{}
	for rows.Next() {
		record, err := scanAttachmentRecord(rows)
		if err != nil {
			return nil, err
		}
		found[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	records := make([]model.AttachmentRecord, 0, len(ids))
	for _, id := range ids {
		record, ok := found[id]
		if !ok {
			return nil, fmt.Errorf("attachment not found: %s", id)
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Store) AttachmentRecordByID(id string) (model.AttachmentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return model.AttachmentRecord{}, fmt.Errorf("attachment id is empty")
	}
	row := s.db.QueryRow(`SELECT prompt, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at FROM attachments WHERE prompt = ? AND id = ?`, s.activePrompt, id)
	record, err := scanAttachmentRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.AttachmentRecord{}, fmt.Errorf("attachment not found: %s", id)
		}
		return model.AttachmentRecord{}, err
	}
	return record, nil
}

func scanAttachmentRecord(rows interface{ Scan(dest ...any) error }) (model.AttachmentRecord, error) {
	var record model.AttachmentRecord
	var createdRaw string
	var text sql.NullString
	if err := rows.Scan(&record.Prompt, &record.ID, &record.SessionID, &record.MessageID, &record.Name, &record.MIMEType, &record.Size, &record.StoragePath, &record.SHA256, &text, &record.Status, &createdRaw); err != nil {
		return model.AttachmentRecord{}, err
	}
	if text.Valid {
		record.TextContent = text.String
	}
	record.CreatedAt = parseDBTime(createdRaw)
	record.HasText = strings.TrimSpace(record.TextContent) != ""
	record.TextBytes = len([]byte(record.TextContent))
	return record, nil
}

func uniqueAttachmentIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func publicAttachments(records []model.AttachmentRecord) []model.Attachment {
	out := make([]model.Attachment, 0, len(records))
	for _, record := range records {
		item := record.Attachment
		item.HasText = strings.TrimSpace(record.TextContent) != ""
		item.TextBytes = len([]byte(record.TextContent))
		out = append(out, item)
	}
	return out
}
