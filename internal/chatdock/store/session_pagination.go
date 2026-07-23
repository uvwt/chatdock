package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/model"
)

type sessionListCursor struct {
	Pinned    int    `json:"p"`
	UpdatedAt string `json:"u"`
	ID        string `json:"i"`
}

type sessionSummaryRow struct {
	Summary   model.SessionSummary
	Pinned    int
	UpdatedAt string
}

func (s *Store) ListSessionPage(filter SessionProjectFilter, cursor string, limit int) ([]model.SessionSummary, string, bool, error) {
	limit = normalizeSessionPageLimit(limit)
	decodedCursor, err := decodeSessionListCursor(cursor)
	if err != nil {
		return nil, "", false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
SELECT
	s.id,
	COALESCE(s.project_id, ''),
	s.title,
	s.pinned,
	s.provider_id,
	s.model,
	s.created_at,
	s.updated_at,
	(SELECT COUNT(*) FROM session_messages m WHERE m.session_id = s.id),
	COALESCE((SELECT m.role FROM session_messages m WHERE m.session_id = s.id ORDER BY m.message_index DESC LIMIT 1), ''),
	COALESCE((SELECT m.content FROM session_messages m WHERE m.session_id = s.id ORDER BY m.message_index DESC LIMIT 1), ''),
	COALESCE((SELECT m.error_json FROM session_messages m WHERE m.session_id = s.id ORDER BY m.message_index DESC LIMIT 1), '')
FROM sessions s
WHERE 1 = 1
  AND NOT EXISTS (
	SELECT 1 FROM scheduled_tasks task
	WHERE task.session_id = s.id
  )
  AND NOT EXISTS (
	SELECT 1 FROM scheduled_task_runs run
	WHERE run.session_id = s.id
  )`
	args := []any{}
	switch filter.Mode {
	case SessionProjectFilterByProject:
		projectID := strings.TrimSpace(filter.ProjectID)
		if projectID == "" {
			return nil, "", false, fmt.Errorf("project id is empty")
		}
		query += ` AND s.project_id = ?`
		args = append(args, projectID)
	case SessionProjectFilterNoProject:
		query += ` AND s.project_id IS NULL`
	default:
	}
	if decodedCursor != nil {
		query += `
  AND (
	s.pinned < ?
	OR (s.pinned = ? AND s.updated_at < ?)
	OR (s.pinned = ? AND s.updated_at = ? AND s.id < ?)
  )`
		args = append(args,
			decodedCursor.Pinned,
			decodedCursor.Pinned, decodedCursor.UpdatedAt,
			decodedCursor.Pinned, decodedCursor.UpdatedAt, decodedCursor.ID,
		)
	}
	query += ` ORDER BY s.pinned DESC, s.updated_at DESC, s.id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()

	pageRows := make([]sessionSummaryRow, 0, limit+1)
	for rows.Next() {
		var row sessionSummaryRow
		var createdAt, lastRole, lastContent, lastError string
		if err := rows.Scan(
			&row.Summary.ID,
			&row.Summary.ProjectID,
			&row.Summary.Title,
			&row.Pinned,
			&row.Summary.ProviderID,
			&row.Summary.Model,
			&createdAt,
			&row.UpdatedAt,
			&row.Summary.Count,
			&lastRole,
			&lastContent,
			&lastError,
		); err != nil {
			return nil, "", false, err
		}
		row.Summary.Pinned = row.Pinned != 0
		row.Summary.CreatedAt = parseDBTimeZero(createdAt)
		row.Summary.UpdatedAt = parseDBTimeZero(row.UpdatedAt)
		row.Summary.LastRole, row.Summary.Preview = sessionSummaryPreview(lastRole, lastContent, lastError)
		pageRows = append(pageRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}

	hasMore := len(pageRows) > limit
	if hasMore {
		pageRows = pageRows[:limit]
	}
	items := make([]model.SessionSummary, len(pageRows))
	for i := range pageRows {
		items[i] = pageRows[i].Summary
	}
	if !hasMore || len(pageRows) == 0 {
		return items, "", false, nil
	}
	last := pageRows[len(pageRows)-1]
	nextCursor, err := encodeSessionListCursor(sessionListCursor{Pinned: last.Pinned, UpdatedAt: last.UpdatedAt, ID: last.Summary.ID})
	if err != nil {
		return nil, "", false, err
	}
	return items, nextCursor, true, nil
}

func normalizeSessionPageLimit(limit int) int {
	if limit <= 0 {
		return 30
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func encodeSessionListCursor(cursor sessionListCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeSessionListCursor(value string) (*sessionListCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid session cursor")
	}
	var cursor sessionListCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || (cursor.Pinned != 0 && cursor.Pinned != 1) || parseDBTimeZero(cursor.UpdatedAt).IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return nil, fmt.Errorf("invalid session cursor")
	}
	return &cursor, nil
}

func sessionSummaryPreview(role string, content string, errorRaw string) (string, string) {
	message := model.Message{Role: role, Content: content}
	if strings.TrimSpace(content) == "" && strings.TrimSpace(errorRaw) != "" && strings.TrimSpace(errorRaw) != "null" {
		var messageError model.MessageError
		if json.Unmarshal([]byte(errorRaw), &messageError) == nil {
			message.Error = &messageError
		}
	}
	return sessionPreview(&model.Session{Messages: []model.Message{message}})
}
