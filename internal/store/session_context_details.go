package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/model"
)

// HydrateMessageEventDetails 只为即将进入模型上下文的消息补载完整事件详情。
// 会话列表和普通历史读取仍保持轻量，避免重新把所有大型工具结果常驻内存。
func (s *Store) HydrateMessageEventDetails(sessionID string, messages []model.Message, messageIndexes []int) error {
	if len(messages) == 0 || len(messageIndexes) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}

	indexes := make([]int, 0, len(messageIndexes))
	seen := make(map[int]bool, len(messageIndexes))
	for _, index := range messageIndexes {
		if index < 0 || index >= len(messages) || seen[index] {
			continue
		}
		seen[index] = true
		indexes = append(indexes, index)
	}
	if len(indexes) == 0 {
		return nil
	}

	placeholders := make([]string, len(indexes))
	args := make([]any, 0, len(indexes)+1)
	args = append(args, sessionID)
	for index, messageIndex := range indexes {
		placeholders[index] = "?"
		args = append(args, messageIndex)
	}
	query := `SELECT e.id, d.details_json
FROM session_message_events e
JOIN session_message_event_details d
  ON d.session_id = e.session_id AND d.event_id = e.id
WHERE e.session_id = ? AND e.message_index IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	detailsByEventID := make(map[string]map[string]any)
	for rows.Next() {
		var eventID, raw string
		if err := rows.Scan(&eventID, &raw); err != nil {
			return err
		}
		if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
			continue
		}
		var details map[string]any
		if err := json.Unmarshal([]byte(raw), &details); err != nil {
			return fmt.Errorf("decode event %s details: %w", eventID, err)
		}
		detailsByEventID[eventID] = details
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, messageIndex := range indexes {
		message := &messages[messageIndex]
		for eventIndex := range message.Events {
			if details := detailsByEventID[message.Events[eventIndex].ID]; len(details) > 0 {
				message.Events[eventIndex].Details = details
			}
		}
	}
	return nil
}
