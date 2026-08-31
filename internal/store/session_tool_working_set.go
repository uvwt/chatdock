package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// SessionToolWorkingSetEntry 只记录会话对工具的近期使用倾向。
// Schema、权限与实际可用性始终由当前 MCP catalog 决定，不能从这里恢复。
type SessionToolWorkingSetEntry struct {
	ToolName           string
	ResourceID         string
	LastDiscoveredTurn int
	LastCalledTurn     int
}

func (s *Store) SessionToolWorkingSet(sessionID string) ([]SessionToolWorkingSetEntry, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT tool_name, resource_id, last_discovered_turn, last_called_turn
FROM session_tool_working_set
WHERE session_id = ?
ORDER BY tool_name`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]SessionToolWorkingSetEntry, 0, 8)
	for rows.Next() {
		var entry SessionToolWorkingSetEntry
		if err := rows.Scan(&entry.ToolName, &entry.ResourceID, &entry.LastDiscoveredTurn, &entry.LastCalledTurn); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) RecordSessionToolDiscovery(sessionID string, turn int, entries []SessionToolWorkingSetEntry) error {
	if turn <= 0 || len(entries) == 0 {
		return nil
	}
	for i := range entries {
		entries[i].LastDiscoveredTurn = turn
	}
	return s.upsertSessionToolWorkingSet(sessionID, entries)
}

func (s *Store) RecordSessionToolCall(sessionID string, turn int, entry SessionToolWorkingSetEntry) error {
	if turn <= 0 {
		return nil
	}
	entry.LastCalledTurn = turn
	return s.upsertSessionToolWorkingSet(sessionID, []SessionToolWorkingSetEntry{entry})
}

func (s *Store) upsertSessionToolWorkingSet(sessionID string, entries []SessionToolWorkingSetEntry) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var currentUserTurns int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM session_messages WHERE session_id = ? AND role = 'user'`, sessionID).Scan(&currentUserTurns); err != nil {
		return err
	}
	for _, entry := range entries {
		toolName := strings.TrimSpace(entry.ToolName)
		resourceID := strings.TrimSpace(entry.ResourceID)
		if toolName == "" || resourceID == "" {
			return fmt.Errorf("tool working set entry is incomplete")
		}
		entryTurn := max(entry.LastDiscoveredTurn, entry.LastCalledTurn)
		if entryTurn > currentUserTurns {
			// 编辑/截断会让会话 turn 回退；旧任务晚到的写入不能重新污染新时间线。
			continue
		}
		if _, err := tx.Exec(`INSERT INTO session_tool_working_set(session_id, tool_name, resource_id, last_discovered_turn, last_called_turn)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(session_id, tool_name) DO UPDATE SET
  resource_id = excluded.resource_id,
  last_discovered_turn = MAX(session_tool_working_set.last_discovered_turn, excluded.last_discovered_turn),
  last_called_turn = MAX(session_tool_working_set.last_called_turn, excluded.last_called_turn)`,
			sessionID, toolName, resourceID, entry.LastDiscoveredTurn, entry.LastCalledTurn); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteSessionToolWorkingSetEntriesIfUnchanged(sessionID string, entries []SessionToolWorkingSetEntry) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`DELETE FROM session_tool_working_set
WHERE session_id = ? AND tool_name = ? AND resource_id = ?
  AND last_discovered_turn = ? AND last_called_turn = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		if _, err := stmt.Exec(sessionID, strings.TrimSpace(entry.ToolName), strings.TrimSpace(entry.ResourceID), entry.LastDiscoveredTurn, entry.LastCalledTurn); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func clearSessionToolWorkingSetWith(tx *sql.Tx, sessionID string) error {
	_, err := tx.Exec(`DELETE FROM session_tool_working_set WHERE session_id = ?`, strings.TrimSpace(sessionID))
	return err
}
