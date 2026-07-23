package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

const maxAttachmentsPerMessage = 20

type AttachmentBlob struct {
	SHA256      string
	StoragePath string
	Size        int64
	MIMEType    string
	RefCount    int
	CreatedAt   time.Time
}

func (s *Store) SaveAttachment(record model.AttachmentRecord) (model.AttachmentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(record.ID) == "" {
		record.ID = model.NewID()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return model.AttachmentRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if strings.TrimSpace(record.SHA256) != "" {
		if _, err := tx.Exec(`INSERT INTO attachment_blobs(sha256, storage_path, size, mime_type, ref_count, created_at) VALUES(?, ?, ?, ?, 0, ?)
ON CONFLICT(sha256) DO UPDATE SET
  storage_path = CASE WHEN attachment_blobs.ref_count = 0 THEN excluded.storage_path ELSE attachment_blobs.storage_path END,
  size = CASE WHEN attachment_blobs.ref_count = 0 THEN excluded.size ELSE attachment_blobs.size END,
  mime_type = CASE WHEN attachment_blobs.ref_count = 0 THEN excluded.mime_type ELSE attachment_blobs.mime_type END,
  created_at = CASE WHEN attachment_blobs.ref_count = 0 THEN excluded.created_at ELSE attachment_blobs.created_at END`, record.SHA256, record.StoragePath, record.Size, record.MIMEType, formatDBTime(record.CreatedAt)); err != nil {
			return model.AttachmentRecord{}, err
		}
		var canonicalPath string
		var canonicalSize int64
		if err := tx.QueryRow(`SELECT storage_path, size FROM attachment_blobs WHERE sha256 = ?`, record.SHA256).Scan(&canonicalPath, &canonicalSize); err != nil {
			return model.AttachmentRecord{}, err
		}
		record.StoragePath = canonicalPath
		record.Size = canonicalSize
	}
	if _, err := tx.Exec(`INSERT INTO attachments(id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, record.ID, record.SessionID, record.MessageID, record.Name, record.MIMEType, record.Size, record.StoragePath, record.SHA256, record.TextContent, record.Status, formatDBTime(record.CreatedAt)); err != nil {
		return model.AttachmentRecord{}, err
	}
	if strings.TrimSpace(record.SHA256) != "" {
		if _, err := tx.Exec(`UPDATE attachment_blobs SET ref_count = (SELECT COUNT(*) FROM attachments WHERE sha256 = ?) WHERE sha256 = ?`, record.SHA256, record.SHA256); err != nil {
			return model.AttachmentRecord{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.AttachmentRecord{}, err
	}
	return record, nil
}

func (s *Store) AttachmentBlobBySHA256(sha string) (AttachmentBlob, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return AttachmentBlob{}, false, nil
	}
	var blob AttachmentBlob
	var createdRaw string
	err := s.db.QueryRow(`SELECT sha256, storage_path, size, mime_type, ref_count, created_at FROM attachment_blobs WHERE sha256 = ?`, sha).Scan(&blob.SHA256, &blob.StoragePath, &blob.Size, &blob.MIMEType, &blob.RefCount, &createdRaw)
	if err == sql.ErrNoRows {
		return AttachmentBlob{}, false, nil
	}
	if err != nil {
		return AttachmentBlob{}, false, err
	}
	blob.CreatedAt = parseDBTime(createdRaw)
	return blob, true, nil
}

func (s *Store) attachmentRecordsByIDsLocked(ids []string) ([]model.AttachmentRecord, error) {
	var err error
	ids, err = normalizeAttachmentIDs(ids)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(`SELECT id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at FROM attachments WHERE id IN (`+placeholders+`)`, args...)
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
	row := s.db.QueryRow(`SELECT id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at FROM attachments WHERE id = ?`, id)
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
	if err := rows.Scan(&record.ID, &record.SessionID, &record.MessageID, &record.Name, &record.MIMEType, &record.Size, &record.StoragePath, &record.SHA256, &text, &record.Status, &createdRaw); err != nil {
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

func normalizeAttachmentIDs(ids []string) ([]string, error) {
	ids = uniqueAttachmentIDs(ids)
	if len(ids) > maxAttachmentsPerMessage {
		return nil, fmt.Errorf("attachment count exceeds %d", maxAttachmentsPerMessage)
	}
	return ids, nil
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
