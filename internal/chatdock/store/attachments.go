package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type AttachmentBlob struct {
	SHA256      string
	StoragePath string
	Size        int64
	MIMEType    string
	RefCount    int
	CreatedAt   time.Time
}

func (s *Store) SaveAttachment(workspaceID string, record model.AttachmentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = model.NewID()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	record.Prompt = workspaceID
	if strings.TrimSpace(record.SHA256) != "" {
		_, _ = s.db.Exec(`INSERT INTO attachment_blobs(sha256, storage_path, size, mime_type, ref_count, created_at) VALUES(?, ?, ?, ?, 0, ?) ON CONFLICT(sha256) DO UPDATE SET ref_count = attachment_blobs.ref_count`, record.SHA256, record.StoragePath, record.Size, record.MIMEType, formatDBTime(record.CreatedAt))
	}
	_, err = s.db.Exec(`INSERT INTO attachments(workspace_id, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, record.Prompt, record.ID, record.SessionID, record.MessageID, record.Name, record.MIMEType, record.Size, record.StoragePath, record.SHA256, record.TextContent, record.Status, formatDBTime(record.CreatedAt))
	if err != nil {
		return err
	}
	if strings.TrimSpace(record.SHA256) != "" {
		_, _ = s.db.Exec(`UPDATE attachment_blobs SET ref_count = (SELECT COUNT(*) FROM attachments WHERE sha256 = ?) WHERE sha256 = ?`, record.SHA256, record.SHA256)
	}
	return nil
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

func (s *Store) migrateAttachmentBlobs() error {
	migrated, err := s.metaValue("attachment_blobs_migrated")
	if err != nil {
		return err
	}
	if migrated == "1" {
		return nil
	}
	rows, err := s.db.Query(`SELECT sha256, storage_path, size, mime_type, created_at FROM attachments WHERE sha256 != '' ORDER BY created_at ASC`)
	if err != nil {
		return err
	}
	type row struct {
		sha, path, mime, created string
		size                     int64
	}
	items := []row{}
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.sha, &item.path, &item.size, &item.mime, &item.created); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.sha] {
			continue
		}
		seen[item.sha] = true
		if _, err := s.db.Exec(`INSERT INTO attachment_blobs(sha256, storage_path, size, mime_type, ref_count, created_at) VALUES(?, ?, ?, ?, 0, ?) ON CONFLICT(sha256) DO NOTHING`, item.sha, item.path, item.size, item.mime, item.created); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`UPDATE attachment_blobs SET ref_count = (SELECT COUNT(*) FROM attachments WHERE attachments.sha256 = attachment_blobs.sha256)`); err != nil {
		return err
	}
	return s.setMetaValue("attachment_blobs_migrated", "1")
}

func (s *Store) attachmentRecordsByIDsLocked(workspaceID string, ids []string) ([]model.AttachmentRecord, error) {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	ids = uniqueAttachmentIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, workspaceID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(`SELECT workspace_id, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at FROM attachments WHERE workspace_id = ? AND id IN (`+placeholders+`)`, args...)
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

func (s *Store) AttachmentRecordByID(workspaceID string, id string) (model.AttachmentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.AttachmentRecord{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return model.AttachmentRecord{}, fmt.Errorf("attachment id is empty")
	}
	row := s.db.QueryRow(`SELECT workspace_id, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at FROM attachments WHERE workspace_id = ? AND id = ?`, workspaceID, id)
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
