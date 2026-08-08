package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/model"
)

func (s *Store) SaveMCPConfirmation(record MCPConfirmationRecord) (MCPConfirmationRecord, error) {
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		record.ID = model.NewID()
	}
	record.SessionID = strings.TrimSpace(record.SessionID)
	record.Tool = strings.TrimSpace(record.Tool)
	record.Status = strings.TrimSpace(record.Status)
	if record.Status == "" {
		record.Status = "pending"
	}
	if record.RequestedAt.IsZero() {
		record.RequestedAt = time.Now()
	}
	argsRaw, err := json.Marshal(record.Arguments)
	if err != nil {
		return MCPConfirmationRecord{}, err
	}
	resolvedRaw := ""
	if record.ResolvedAt != nil {
		resolvedRaw = formatDBTime(*record.ResolvedAt)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO mcp_confirmations(id, session_id, tool, arguments_json, status, requested_at, resolved_at, message) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, tool = excluded.tool, arguments_json = excluded.arguments_json, status = excluded.status, requested_at = excluded.requested_at, resolved_at = excluded.resolved_at, message = excluded.message`, record.ID, record.SessionID, record.Tool, string(argsRaw), record.Status, formatDBTime(record.RequestedAt), resolvedRaw, record.Message)
	if err != nil {
		return MCPConfirmationRecord{}, err
	}
	return record, nil
}

func (s *Store) ResolveMCPConfirmation(id string, status string, approved bool, resolvedAt time.Time) (MCPConfirmationRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MCPConfirmationRecord{}, fmt.Errorf("mcp confirmation id is empty")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		if approved {
			status = "approved"
		} else {
			status = "denied"
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var existingStatus string
	if err := s.db.QueryRow(`SELECT status FROM mcp_confirmations WHERE id = ?`, id).Scan(&existingStatus); err != nil {
		if err == sql.ErrNoRows {
			return MCPConfirmationRecord{}, fmt.Errorf("mcp confirmation not found")
		}
		return MCPConfirmationRecord{}, err
	}
	if existingStatus == "pending" {
		if _, err := s.db.Exec(`UPDATE mcp_confirmations SET status = ?, resolved_at = ? WHERE id = ?`, status, formatDBTime(resolvedAt), id); err != nil {
			return MCPConfirmationRecord{}, err
		}
	}
	return scanMCPConfirmationRow(s.db.QueryRow(`SELECT id, session_id, tool, arguments_json, status, requested_at, resolved_at, message FROM mcp_confirmations WHERE id = ?`, id))
}

func (s *Store) ListMCPConfirmations(includeResolved bool, limit int) ([]MCPConfirmationRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	query := `SELECT id, session_id, tool, arguments_json, status, requested_at, resolved_at, message FROM mcp_confirmations WHERE 1 = 1`
	args := []any{}
	if !includeResolved {
		query += ` AND status = 'pending'`
	}
	query += ` ORDER BY requested_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MCPConfirmationRecord{}
	for rows.Next() {
		item, err := scanMCPConfirmationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanMCPConfirmationRow(row interface{ Scan(dest ...any) error }) (MCPConfirmationRecord, error) {
	var item MCPConfirmationRecord
	var argsRaw, requestedRaw, resolvedRaw string
	if err := row.Scan(&item.ID, &item.SessionID, &item.Tool, &argsRaw, &item.Status, &requestedRaw, &resolvedRaw, &item.Message); err != nil {
		return MCPConfirmationRecord{}, err
	}
	if strings.TrimSpace(argsRaw) != "" {
		if err := json.Unmarshal([]byte(argsRaw), &item.Arguments); err != nil {
			return MCPConfirmationRecord{}, fmt.Errorf("decode MCP confirmation %s arguments: %w", item.ID, err)
		}
	}
	item.RequestedAt = parseDBTimeZero(requestedRaw)
	if strings.TrimSpace(resolvedRaw) != "" {
		resolved := parseDBTimeZero(resolvedRaw)
		item.ResolvedAt = &resolved
	}
	return item, nil
}
