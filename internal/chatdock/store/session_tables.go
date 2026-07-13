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
			return fmt.Errorf("decode legacy session %s: %w", item.id, err)
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
	sessions, err := loadSessionHeadersFromTables(db, prompt)
	if err != nil {
		return nil, err
	}
	if err := loadSessionMessagesFromTables(db, prompt, sessions); err != nil {
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

func loadSessionHeadersFromTables(db *sql.DB, prompt string) (map[string]*model.Session, error) {
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
	return sessions, rows.Err()
}

func loadSessionMessagesFromTables(db *sql.DB, prompt string, sessions map[string]*model.Session) error {
	rows, err := db.Query(`SELECT session_id, message_index, id, role, content, reasoning, error_json, attachments_json, created_at FROM session_messages WHERE workspace_id = ? ORDER BY session_id, message_index`, prompt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sessionID, errorRaw, attachmentsRaw, createdAt string
		var index int
		var msg model.Message
		if err := rows.Scan(&sessionID, &index, &msg.ID, &msg.Role, &msg.Content, &msg.Reasoning, &errorRaw, &attachmentsRaw, &createdAt); err != nil {
			return err
		}
		if index < 0 {
			return fmt.Errorf("session %s has negative message index %d", sessionID, index)
		}
		session := sessions[sessionID]
		if session == nil {
			return fmt.Errorf("message %s references missing session %s", msg.ID, sessionID)
		}
		msg.CreatedAt = parseDBTimeZero(createdAt)
		if strings.TrimSpace(errorRaw) != "" && strings.TrimSpace(errorRaw) != "null" {
			var messageError model.MessageError
			if err := json.Unmarshal([]byte(errorRaw), &messageError); err != nil {
				return fmt.Errorf("decode session %s message %d error: %w", sessionID, index, err)
			}
			msg.Error = &messageError
		}
		if strings.TrimSpace(attachmentsRaw) != "" {
			if err := json.Unmarshal([]byte(attachmentsRaw), &msg.Attachments); err != nil {
				return fmt.Errorf("decode session %s message %d attachments: %w", sessionID, index, err)
			}
		}
		for len(session.Messages) <= index {
			session.Messages = append(session.Messages, model.Message{})
		}
		session.Messages[index] = msg
	}
	return rows.Err()
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
		if messageIndex < 0 || eventIndex < 0 {
			return fmt.Errorf("session %s event %s has invalid indexes message=%d event=%d", sessionID, event.ID, messageIndex, eventIndex)
		}
		session := sessions[sessionID]
		if session == nil {
			return fmt.Errorf("event %s references missing session %s", event.ID, sessionID)
		}
		if messageIndex >= len(session.Messages) {
			return fmt.Errorf("event %s references missing message %d in session %s", event.ID, messageIndex, sessionID)
		}
		for len(session.Messages[messageIndex].Events) <= eventIndex {
			session.Messages[messageIndex].Events = append(session.Messages[messageIndex].Events, model.MessageEvent{})
		}
		session.Messages[messageIndex].Events[eventIndex] = event
	}
	return rows.Err()
}

func loadSessionPartsFromTables(db *sql.DB, prompt string, sessions map[string]*model.Session) error {
	rows, err := db.Query(`SELECT session_id, message_index, part_index, kind, text, call_key, event_id FROM session_message_parts WHERE workspace_id = ? ORDER BY session_id, message_index, part_index`, prompt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sessionID string
		var messageIndex, partIndex int
		var part model.MessagePart
		var eventID string
		if err := rows.Scan(&sessionID, &messageIndex, &partIndex, &part.Kind, &part.Text, &part.CallKey, &eventID); err != nil {
			return err
		}
		if messageIndex < 0 || partIndex < 0 {
			return fmt.Errorf("session %s part has invalid indexes message=%d part=%d", sessionID, messageIndex, partIndex)
		}
		session := sessions[sessionID]
		if session == nil {
			return fmt.Errorf("message part references missing session %s", sessionID)
		}
		if messageIndex >= len(session.Messages) {
			return fmt.Errorf("message part references missing message %d in session %s", messageIndex, sessionID)
		}
		if strings.TrimSpace(eventID) != "" {
			event, ok := messageEventByID(session.Messages[messageIndex].Events, eventID)
			if !ok {
				return fmt.Errorf("message part references missing event %s in session %s", eventID, sessionID)
			}
			part.Event = &event
		}
		for len(session.Messages[messageIndex].Parts) <= partIndex {
			session.Messages[messageIndex].Parts = append(session.Messages[messageIndex].Parts, model.MessagePart{})
		}
		session.Messages[messageIndex].Parts[partIndex] = part
	}
	return rows.Err()
}

func messageEventByID(events []model.MessageEvent, eventID string) (model.MessageEvent, bool) {
	for _, event := range events {
		if event.ID == eventID {
			return event, true
		}
	}
	return model.MessageEvent{}, false
}

func upsertSessionTablesTx(tx sqlWriter, prompt string, session *model.Session) error {
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
	compactRaw, err := json.MarshalIndent(compactSessionForLegacyRow(session), "", "  ")
	if err != nil {
		return fmt.Errorf("encode compact session %s: %w", session.ID, err)
	}
	if _, err := tx.Exec(`INSERT INTO sessions(workspace_id, id, title, pinned, provider_id, model, json, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, id) DO UPDATE SET title = excluded.title, pinned = excluded.pinned, provider_id = excluded.provider_id, model = excluded.model, json = excluded.json, created_at = excluded.created_at, updated_at = excluded.updated_at`, prompt, session.ID, session.Title, boolInt(session.Pinned), session.ProviderID, session.Model, string(compactRaw)+"\n", formatScheduleDBTime(session.CreatedAt), formatScheduleDBTime(session.UpdatedAt)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM session_messages WHERE workspace_id = ? AND session_id = ?`, prompt, session.ID); err != nil {
		return err
	}
	for messageIndex := range session.Messages {
		if err := upsertSessionMessageTx(tx, prompt, session, messageIndex); err != nil {
			return err
		}
	}
	return nil
}

func upsertSessionMessageTx(tx sqlWriter, prompt string, session *model.Session, messageIndex int) error {
	msg := &session.Messages[messageIndex]
	if strings.TrimSpace(msg.ID) == "" {
		msg.ID = model.NewID()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = session.UpdatedAt
	}
	attachmentsRaw, err := json.Marshal(msg.Attachments)
	if err != nil {
		return fmt.Errorf("encode session %s message %d attachments: %w", session.ID, messageIndex, err)
	}
	errorRaw := ""
	if msg.Error != nil {
		encodedError, err := json.Marshal(msg.Error)
		if err != nil {
			return fmt.Errorf("encode session %s message %d error: %w", session.ID, messageIndex, err)
		}
		errorRaw = string(encodedError)
	}
	if _, err := tx.Exec(`INSERT INTO session_messages(workspace_id, session_id, message_index, id, role, content, reasoning, error_json, attachments_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, prompt, session.ID, messageIndex, msg.ID, msg.Role, msg.Content, msg.Reasoning, errorRaw, string(attachmentsRaw), formatScheduleDBTime(msg.CreatedAt)); err != nil {
		return err
	}
	eventIDs, err := upsertSessionMessageEventsTx(tx, prompt, session.ID, messageIndex, msg)
	if err != nil {
		return err
	}
	return upsertSessionMessagePartsTx(tx, prompt, session.ID, messageIndex, msg, eventIDs)
}

func upsertSessionMessageEventsTx(tx sqlWriter, prompt string, sessionID string, messageIndex int, msg *model.Message) (map[int]string, error) {
	eventIDs := make(map[int]string, len(msg.Events))
	for eventIndex := range msg.Events {
		event := &msg.Events[eventIndex]
		ensureMessageEventID(sessionID, msg.ID, messageIndex, eventIndex, event)
		eventIDs[eventIndex] = event.ID
		if err := insertSessionMessageEventTx(tx, prompt, sessionID, messageIndex, eventIndex, *event); err != nil {
			return nil, err
		}
	}
	return eventIDs, nil
}

func upsertSessionMessagePartsTx(tx sqlWriter, prompt string, sessionID string, messageIndex int, msg *model.Message, eventIDs map[int]string) error {
	for partIndex := range msg.Parts {
		part := &msg.Parts[partIndex]
		eventID, err := resolveSessionPartEventID(tx, prompt, sessionID, messageIndex, partIndex, msg, eventIDs)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO session_message_parts(workspace_id, session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, prompt, sessionID, messageIndex, partIndex, part.Kind, part.Text, part.CallKey, eventID); err != nil {
			return err
		}
	}
	return nil
}

func resolveSessionPartEventID(tx sqlWriter, prompt string, sessionID string, messageIndex int, partIndex int, msg *model.Message, eventIDs map[int]string) (string, error) {
	part := &msg.Parts[partIndex]
	if part.Event == nil {
		return "", nil
	}
	if match := matchingMessageEventIndex(msg.Events, *part.Event); match >= 0 {
		return eventIDs[match], nil
	}
	eventIndex := len(msg.Events) + partIndex
	ensureMessageEventID(sessionID, msg.ID, messageIndex, eventIndex, part.Event)
	if err := insertSessionMessageEventTx(tx, prompt, sessionID, messageIndex, eventIndex, *part.Event); err != nil {
		return "", err
	}
	return part.Event.ID, nil
}

func insertSessionMessageEventTx(tx sqlWriter, prompt string, sessionID string, messageIndex int, eventIndex int, event model.MessageEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = model.NewID()
	}
	meta := strings.TrimSpace(event.Meta)
	if meta == "" && len(event.Details) > 0 {
		meta = compactEventMetaForDB(event.Details)
	}
	if _, err := tx.Exec(`INSERT INTO session_message_events(workspace_id, session_id, message_index, event_index, id, kind, phase, call_key, text, meta) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, prompt, sessionID, messageIndex, eventIndex, event.ID, event.Kind, event.Phase, event.CallKey, event.Text, meta); err != nil {
		return err
	}
	if len(event.Details) > 0 {
		detailsRaw, err := json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("encode session %s event %s details: %w", sessionID, event.ID, err)
		}
		if _, err := tx.Exec(`INSERT INTO session_message_event_details(workspace_id, session_id, event_id, details_json, details_bytes, updated_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(workspace_id, session_id, event_id) DO UPDATE SET details_json = excluded.details_json, details_bytes = excluded.details_bytes, updated_at = excluded.updated_at`, prompt, sessionID, event.ID, string(detailsRaw), len(detailsRaw), formatScheduleDBTime(time.Now())); err != nil {
			return err
		}
	}
	return nil
}

func compactEventMetaForDB(details map[string]any) string {
	out := map[string]any{}
	if args, ok := details["arguments"].(map[string]any); ok {
		argOut := map[string]any{}
		for _, key := range []string{"name", "tool", "query"} {
			if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
				argOut[key] = strings.TrimSpace(value)
			}
		}
		if len(argOut) > 0 {
			out["arguments"] = argOut
		}
	}
	if value, ok := details["tool"].(string); ok && strings.TrimSpace(value) != "" {
		out["tool"] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return ""
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(raw)
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

func (s *Store) SessionMessageEventByID(workspaceID string, sessionID string, eventID string) (model.MessageEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.MessageEvent{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	eventID = strings.TrimSpace(eventID)
	if sessionID == "" || eventID == "" {
		return model.MessageEvent{}, fmt.Errorf("session id and event id are required")
	}
	row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.workspace_id = e.workspace_id AND d.session_id = e.session_id AND d.event_id = e.id WHERE e.workspace_id = ? AND e.session_id = ? AND e.id = ?`, workspaceID, sessionID, eventID)
	return scanSessionMessageEventWithDetails(row)
}

func (s *Store) SessionMessageEventByIndex(workspaceID string, sessionID string, messageIndex int, eventIndex int, partIndex int) (model.MessageEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.MessageEvent{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || messageIndex < 0 {
		return model.MessageEvent{}, fmt.Errorf("invalid session event ref")
	}
	if eventIndex < 0 && partIndex >= 0 {
		var eventID string
		if err := s.db.QueryRow(`SELECT event_id FROM session_message_parts WHERE workspace_id = ? AND session_id = ? AND message_index = ? AND part_index = ?`, workspaceID, sessionID, messageIndex, partIndex).Scan(&eventID); err != nil {
			return model.MessageEvent{}, err
		}
		eventIndex = -1
		row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.workspace_id = e.workspace_id AND d.session_id = e.session_id AND d.event_id = e.id WHERE e.workspace_id = ? AND e.session_id = ? AND e.id = ?`, workspaceID, sessionID, eventID)
		return scanSessionMessageEventWithDetails(row)
	}
	if eventIndex < 0 {
		return model.MessageEvent{}, fmt.Errorf("invalid event index")
	}
	row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.workspace_id = e.workspace_id AND d.session_id = e.session_id AND d.event_id = e.id WHERE e.workspace_id = ? AND e.session_id = ? AND e.message_index = ? AND e.event_index = ?`, workspaceID, sessionID, messageIndex, eventIndex)
	return scanSessionMessageEventWithDetails(row)
}

func scanSessionMessageEventWithDetails(scanner interface{ Scan(...any) error }) (model.MessageEvent, error) {
	var event model.MessageEvent
	var detailsRaw string
	if err := scanner.Scan(&event.ID, &event.Kind, &event.Phase, &event.CallKey, &event.Text, &event.Meta, &detailsRaw); err != nil {
		return model.MessageEvent{}, err
	}
	if strings.TrimSpace(detailsRaw) == "" {
		return event, nil
	}
	if err := json.Unmarshal([]byte(detailsRaw), &event.Details); err != nil {
		return model.MessageEvent{}, fmt.Errorf("decode session event %s details: %w", event.ID, err)
	}
	return event, nil
}
