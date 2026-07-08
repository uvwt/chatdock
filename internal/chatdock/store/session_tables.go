package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

const sessionTablesMigratedKey = "session_tables_migrated"

func (s *Store) migrateSessionJSONToTables() error {
	migrated, err := s.metaValue(sessionTablesMigratedKey)
	if err != nil {
		return err
	}
	if migrated == "1" {
		return nil
	}
	rows, err := s.db.Query(`SELECT workspace_id, id, json, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return err
	}
	type row struct {
		prompt, id, raw, createdAt, updatedAt string
	}
	legacy := []row{}
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.prompt, &item.id, &item.raw, &item.createdAt, &item.updatedAt); err != nil {
			_ = rows.Close()
			return err
		}
		legacy = append(legacy, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range legacy {
		if strings.TrimSpace(item.raw) == "" || strings.TrimSpace(item.raw) == "{}" {
			continue
		}
		var session model.Session
		if err := json.Unmarshal([]byte(item.raw), &session); err != nil {
			continue
		}
		if session.ID == "" {
			session.ID = item.id
		}
		if session.CreatedAt.IsZero() {
			session.CreatedAt = parseDBTimeZero(item.createdAt)
		}
		if session.UpdatedAt.IsZero() {
			session.UpdatedAt = parseDBTimeZero(item.updatedAt)
		}
		if err := upsertSessionTablesTx(tx, item.prompt, &session); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES(?, '1') ON CONFLICT(key) DO UPDATE SET value = '1'`, sessionTablesMigratedKey); err != nil {
		return err
	}
	return tx.Commit()
}

func loadSessionsFromTablesLocked(db *sql.DB, prompt string) (map[string]*model.Session, error) {
	rows, err := db.Query(`SELECT id, title, pinned, provider_id, model, created_at, updated_at FROM sessions WHERE workspace_id = ? ORDER BY updated_at DESC`, prompt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := map[string]*model.Session{}
	for rows.Next() {
		var session model.Session
		var pinned int
		var createdAt, updatedAt string
		if err := rows.Scan(&session.ID, &session.Title, &pinned, &session.ProviderID, &session.Model, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		session.Pinned = pinned != 0
		session.CreatedAt = parseDBTimeZero(createdAt)
		session.UpdatedAt = parseDBTimeZero(updatedAt)
		session.Messages = []model.Message{}
		sessions[session.ID] = &session
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	msgRows, err := db.Query(`SELECT session_id, message_index, id, role, content, reasoning, attachments_json, created_at FROM session_messages WHERE workspace_id = ? ORDER BY session_id, message_index`, prompt)
	if err != nil {
		return nil, err
	}
	for msgRows.Next() {
		var sessionID, attachmentsRaw, createdAt string
		var index int
		var msg model.Message
		if err := msgRows.Scan(&sessionID, &index, &msg.ID, &msg.Role, &msg.Content, &msg.Reasoning, &attachmentsRaw, &createdAt); err != nil {
			_ = msgRows.Close()
			return nil, err
		}
		msg.CreatedAt = parseDBTimeZero(createdAt)
		if strings.TrimSpace(attachmentsRaw) != "" {
			_ = json.Unmarshal([]byte(attachmentsRaw), &msg.Attachments)
		}
		session := sessions[sessionID]
		if session == nil {
			continue
		}
		for len(session.Messages) <= index {
			session.Messages = append(session.Messages, model.Message{})
		}
		session.Messages[index] = msg
	}
	if err := msgRows.Close(); err != nil {
		return nil, err
	}
	if err := msgRows.Err(); err != nil {
		return nil, err
	}
	if err := loadSessionEventsFromTables(db, prompt, sessions); err != nil {
		return nil, err
	}
	if err := loadSessionPartsFromTables(db, prompt, sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func loadSessionEventsFromTables(db *sql.DB, prompt string, sessions map[string]*model.Session) error {
	rows, err := db.Query(`SELECT session_id, message_index, event_index, id, kind, phase, call_key, text, meta FROM session_message_events WHERE workspace_id = ? ORDER BY session_id, message_index, event_index`, prompt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sessionID string
		var messageIndex, eventIndex int
		var event model.MessageEvent
		if err := rows.Scan(&sessionID, &messageIndex, &eventIndex, &event.ID, &event.Kind, &event.Phase, &event.CallKey, &event.Text, &event.Meta); err != nil {
			return err
		}
		session := sessions[sessionID]
		if session == nil || messageIndex < 0 || messageIndex >= len(session.Messages) {
			continue
		}
		for len(session.Messages[messageIndex].Events) <= eventIndex {
			session.Messages[messageIndex].Events = append(session.Messages[messageIndex].Events, model.MessageEvent{})
		}
		session.Messages[messageIndex].Events[eventIndex] = event
	}
	return rows.Err()
}

func loadSessionPartsFromTables(db *sql.DB, prompt string, sessions map[string]*model.Session) error {
	rows, err := db.Query(`SELECT workspace_id, session_id, message_index, part_index, kind, text, call_key, event_id FROM session_message_parts WHERE workspace_id = ? ORDER BY session_id, message_index, part_index`, prompt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ignoredPrompt, sessionID string
		var messageIndex, partIndex int
		var part model.MessagePart
		var eventID string
		if err := rows.Scan(&ignoredPrompt, &sessionID, &messageIndex, &partIndex, &part.Kind, &part.Text, &part.CallKey, &eventID); err != nil {
			return err
		}
		_ = ignoredPrompt
		session := sessions[sessionID]
		if session == nil || messageIndex < 0 || messageIndex >= len(session.Messages) {
			continue
		}
		if strings.TrimSpace(eventID) != "" {
			for i := range session.Messages[messageIndex].Events {
				if session.Messages[messageIndex].Events[i].ID == eventID {
					event := session.Messages[messageIndex].Events[i]
					part.Event = &event
					break
				}
			}
		}
		for len(session.Messages[messageIndex].Parts) <= partIndex {
			session.Messages[messageIndex].Parts = append(session.Messages[messageIndex].Parts, model.MessagePart{})
		}
		session.Messages[messageIndex].Parts[partIndex] = part
	}
	return rows.Err()
}

func upsertSessionTablesTx(tx interface {
	Exec(string, ...any) (sql.Result, error)
}, prompt string, session *model.Session) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = defaultWorkspaceID
	}
	now := time.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	if session.Title == "" {
		session.Title = "新会话"
	}
	compactRaw, _ := json.MarshalIndent(compactSessionForLegacyRow(session), "", "  ")
	if _, err := tx.Exec(`INSERT INTO sessions(workspace_id, id, title, pinned, provider_id, model, json, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, id) DO UPDATE SET title = excluded.title, pinned = excluded.pinned, provider_id = excluded.provider_id, model = excluded.model, json = excluded.json, created_at = excluded.created_at, updated_at = excluded.updated_at`, prompt, session.ID, session.Title, boolInt(session.Pinned), session.ProviderID, session.Model, string(compactRaw)+"\n", formatScheduleDBTime(session.CreatedAt), formatScheduleDBTime(session.UpdatedAt)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM session_messages WHERE workspace_id = ? AND session_id = ?`, prompt, session.ID); err != nil {
		return err
	}
	for messageIndex := range session.Messages {
		msg := &session.Messages[messageIndex]
		if strings.TrimSpace(msg.ID) == "" {
			msg.ID = model.NewID()
		}
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = session.UpdatedAt
		}
		attachmentsRaw, _ := json.Marshal(msg.Attachments)
		if _, err := tx.Exec(`INSERT INTO session_messages(workspace_id, session_id, message_index, id, role, content, reasoning, attachments_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, prompt, session.ID, messageIndex, msg.ID, msg.Role, msg.Content, msg.Reasoning, string(attachmentsRaw), formatScheduleDBTime(msg.CreatedAt)); err != nil {
			return err
		}
		eventIDs := map[int]string{}
		for eventIndex := range msg.Events {
			event := &msg.Events[eventIndex]
			ensureMessageEventID(session.ID, msg.ID, messageIndex, eventIndex, event)
			eventIDs[eventIndex] = event.ID
			if err := insertSessionMessageEventTx(tx, prompt, session.ID, messageIndex, eventIndex, *event); err != nil {
				return err
			}
		}
		for partIndex := range msg.Parts {
			part := &msg.Parts[partIndex]
			eventID := ""
			if part.Event != nil {
				match := matchingMessageEventIndex(msg.Events, *part.Event)
				if match >= 0 {
					eventID = eventIDs[match]
				} else {
					ensureMessageEventID(session.ID, msg.ID, messageIndex, len(msg.Events)+partIndex, part.Event)
					eventID = part.Event.ID
					if err := insertSessionMessageEventTx(tx, prompt, session.ID, messageIndex, len(msg.Events)+partIndex, *part.Event); err != nil {
						return err
					}
				}
			}
			if _, err := tx.Exec(`INSERT INTO session_message_parts(workspace_id, session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, prompt, session.ID, messageIndex, partIndex, part.Kind, part.Text, part.CallKey, eventID); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertSessionMessageEventTx(tx interface {
	Exec(string, ...any) (sql.Result, error)
}, prompt string, sessionID string, messageIndex int, eventIndex int, event model.MessageEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = model.NewID()
	}
	if _, err := tx.Exec(`INSERT INTO session_message_events(workspace_id, session_id, message_index, event_index, id, kind, phase, call_key, text, meta) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, prompt, sessionID, messageIndex, eventIndex, event.ID, event.Kind, event.Phase, event.CallKey, event.Text, event.Meta); err != nil {
		return err
	}
	if len(event.Details) > 0 {
		detailsRaw, _ := json.Marshal(event.Details)
		if _, err := tx.Exec(`INSERT INTO session_message_event_details(workspace_id, session_id, event_id, details_json, details_bytes, updated_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(workspace_id, session_id, event_id) DO UPDATE SET details_json = excluded.details_json, details_bytes = excluded.details_bytes, updated_at = excluded.updated_at`, prompt, sessionID, event.ID, string(detailsRaw), len(detailsRaw), formatScheduleDBTime(time.Now())); err != nil {
			return err
		}
	}
	return nil
}

func compactSessionForLegacyRow(session *model.Session) model.Session {
	out := *session
	out.Messages = nil
	return out
}

func ensureMessageEventID(sessionID string, messageID string, messageIndex int, eventIndex int, event *model.MessageEvent) {
	if event == nil || strings.TrimSpace(event.ID) != "" {
		return
	}
	seed := fmt.Sprintf("%s:%s:%d:%d:%s:%s:%s", sessionID, messageID, messageIndex, eventIndex, event.Kind, event.Phase, event.CallKey)
	event.ID = "evt_" + shortStableHash(seed)
}

func shortStableHash(value string) string {
	// FNV is enough here: event IDs only need to be stable within a migrated session.
	var h uint64 = 1469598103934665603
	for i := 0; i < len(value); i++ {
		h ^= uint64(value[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("%016x", h)
}

func matchingMessageEventIndex(events []model.MessageEvent, target model.MessageEvent) int {
	if strings.TrimSpace(target.ID) != "" {
		for i, event := range events {
			if event.ID == target.ID {
				return i
			}
		}
	}
	if strings.TrimSpace(target.CallKey) != "" {
		for i, event := range events {
			if event.CallKey == target.CallKey && event.Kind == target.Kind {
				return i
			}
		}
	}
	for i, event := range events {
		if event.Kind == target.Kind && event.Phase == target.Phase && event.Text == target.Text && event.Meta == target.Meta {
			return i
		}
	}
	return -1
}

func (s *Store) SessionMessageEventByID(sessionID string, eventID string) (model.MessageEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sessionID = strings.TrimSpace(sessionID)
	eventID = strings.TrimSpace(eventID)
	if sessionID == "" || eventID == "" {
		return model.MessageEvent{}, fmt.Errorf("session id and event id are required")
	}
	row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.workspace_id = e.workspace_id AND d.session_id = e.session_id AND d.event_id = e.id WHERE e.workspace_id = ? AND e.session_id = ? AND e.id = ?`, s.workspaceCacheID, sessionID, eventID)
	var event model.MessageEvent
	var detailsRaw string
	if err := row.Scan(&event.ID, &event.Kind, &event.Phase, &event.CallKey, &event.Text, &event.Meta, &detailsRaw); err != nil {
		return model.MessageEvent{}, err
	}
	if strings.TrimSpace(detailsRaw) != "" {
		_ = json.Unmarshal([]byte(detailsRaw), &event.Details)
	}
	return event, nil
}

func (s *Store) SessionMessageEventByIndex(sessionID string, messageIndex int, eventIndex int, partIndex int) (model.MessageEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || messageIndex < 0 {
		return model.MessageEvent{}, fmt.Errorf("invalid session event ref")
	}
	if eventIndex < 0 && partIndex >= 0 {
		var eventID string
		if err := s.db.QueryRow(`SELECT event_id FROM session_message_parts WHERE workspace_id = ? AND session_id = ? AND message_index = ? AND part_index = ?`, s.workspaceCacheID, sessionID, messageIndex, partIndex).Scan(&eventID); err != nil {
			return model.MessageEvent{}, err
		}
		eventIndex = -1
		row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.workspace_id = e.workspace_id AND d.session_id = e.session_id AND d.event_id = e.id WHERE e.workspace_id = ? AND e.session_id = ? AND e.id = ?`, s.workspaceCacheID, sessionID, eventID)
		var event model.MessageEvent
		var detailsRaw string
		if err := row.Scan(&event.ID, &event.Kind, &event.Phase, &event.CallKey, &event.Text, &event.Meta, &detailsRaw); err != nil {
			return model.MessageEvent{}, err
		}
		if strings.TrimSpace(detailsRaw) != "" {
			_ = json.Unmarshal([]byte(detailsRaw), &event.Details)
		}
		return event, nil
	}
	if eventIndex < 0 {
		return model.MessageEvent{}, fmt.Errorf("invalid event index")
	}
	row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.workspace_id = e.workspace_id AND d.session_id = e.session_id AND d.event_id = e.id WHERE e.workspace_id = ? AND e.session_id = ? AND e.message_index = ? AND e.event_index = ?`, s.workspaceCacheID, sessionID, messageIndex, eventIndex)
	var event model.MessageEvent
	var detailsRaw string
	if err := row.Scan(&event.ID, &event.Kind, &event.Phase, &event.CallKey, &event.Text, &event.Meta, &detailsRaw); err != nil {
		return model.MessageEvent{}, err
	}
	if strings.TrimSpace(detailsRaw) != "" {
		_ = json.Unmarshal([]byte(detailsRaw), &event.Details)
	}
	return event, nil
}
