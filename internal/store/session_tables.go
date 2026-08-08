package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/model"
)

func loadSessionsFromTablesLocked(db *sql.DB) (map[string]*model.Session, error) {
	sessions, err := loadSessionHeadersFromTables(db)
	if err != nil {
		return nil, err
	}
	if err := loadSessionMessagesFromTables(db, sessions); err != nil {
		return nil, err
	}
	if err := loadSessionEventsFromTables(db, sessions); err != nil {
		return nil, err
	}
	if err := loadSessionPartsFromTables(db, sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func loadSessionHeadersFromTables(db *sql.DB) (map[string]*model.Session, error) {
	rows, err := db.Query(`SELECT id, project_id, title, pinned, provider_id, model, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := map[string]*model.Session{}
	for rows.Next() {
		var session model.Session
		var pinned int
		var projectID sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&session.ID, &projectID, &session.Title, &pinned, &session.ProviderID, &session.Model, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if projectID.Valid {
			session.ProjectID = projectID.String
		}
		session.Pinned = pinned != 0
		session.CreatedAt = parseDBTimeZero(createdAt)
		session.UpdatedAt = parseDBTimeZero(updatedAt)
		session.Messages = []model.Message{}
		sessions[session.ID] = &session
	}
	return sessions, rows.Err()
}

func loadSessionMessagesFromTables(db *sql.DB, sessions map[string]*model.Session) error {
	rows, err := db.Query(`SELECT session_id, message_index, id, role, content, reasoning, error_json, attachments_json, created_at FROM session_messages ORDER BY session_id, message_index`)
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

func loadSessionEventsFromTables(db *sql.DB, sessions map[string]*model.Session) error {
	rows, err := db.Query(`SELECT session_id, message_index, event_index, id, kind, phase, call_key, text, meta FROM session_message_events ORDER BY session_id, message_index, event_index`)
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

func loadSessionPartsFromTables(db *sql.DB, sessions map[string]*model.Session) error {
	rows, err := db.Query(`SELECT session_id, message_index, part_index, kind, text, call_key, event_id FROM session_message_parts ORDER BY session_id, message_index, part_index`)
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

func upsertSessionTablesTx(tx sqlWriter, session *model.Session) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	session.ProjectID = strings.TrimSpace(session.ProjectID)
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
	var projectID any
	if session.ProjectID != "" {
		projectID = session.ProjectID
	}
	if _, err := tx.Exec(`INSERT INTO sessions(id, project_id, title, pinned, provider_id, model, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET project_id = excluded.project_id, title = excluded.title, pinned = excluded.pinned, provider_id = excluded.provider_id, model = excluded.model, created_at = excluded.created_at, updated_at = excluded.updated_at`, session.ID, projectID, session.Title, boolInt(session.Pinned), session.ProviderID, session.Model, formatScheduleDBTime(session.CreatedAt), formatScheduleDBTime(session.UpdatedAt)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM session_messages WHERE session_id = ?`, session.ID); err != nil {
		return err
	}
	for messageIndex := range session.Messages {
		if err := upsertSessionMessageTx(tx, session, messageIndex); err != nil {
			return err
		}
	}
	// Event details are lazy-loaded and may not be present in the in-memory
	// session. Preserve details whose event IDs were reinserted, and remove only
	// rows whose events disappeared during this replacement.
	if _, err := tx.Exec(`DELETE FROM session_message_event_details
WHERE session_id = ?
  AND NOT EXISTS (
    SELECT 1
    FROM session_message_events AS event
    WHERE event.session_id = session_message_event_details.session_id
      AND event.id = session_message_event_details.event_id
  )`, session.ID); err != nil {
		return err
	}
	return nil
}

func upsertSessionMessageTx(tx sqlWriter, session *model.Session, messageIndex int) error {
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
	if _, err := tx.Exec(`INSERT INTO session_messages(session_id, message_index, id, role, content, reasoning, error_json, attachments_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, session.ID, messageIndex, msg.ID, msg.Role, msg.Content, msg.Reasoning, errorRaw, string(attachmentsRaw), formatScheduleDBTime(msg.CreatedAt)); err != nil {
		return err
	}
	eventIDs, err := upsertSessionMessageEventsTx(tx, session.ID, messageIndex, msg)
	if err != nil {
		return err
	}
	return upsertSessionMessagePartsTx(tx, session.ID, messageIndex, msg, eventIDs)
}

func upsertSessionMessageEventsTx(tx sqlWriter, sessionID string, messageIndex int, msg *model.Message) (map[int]string, error) {
	eventIDs := make(map[int]string, len(msg.Events))
	for eventIndex := range msg.Events {
		event := &msg.Events[eventIndex]
		ensureMessageEventID(sessionID, msg.ID, messageIndex, eventIndex, event)
		eventIDs[eventIndex] = event.ID
		if err := insertSessionMessageEventTx(tx, sessionID, messageIndex, eventIndex, *event); err != nil {
			return nil, err
		}
	}
	return eventIDs, nil
}

func upsertSessionMessagePartsTx(tx sqlWriter, sessionID string, messageIndex int, msg *model.Message, eventIDs map[int]string) error {
	for partIndex := range msg.Parts {
		part := &msg.Parts[partIndex]
		eventID, err := resolveSessionPartEventID(tx, sessionID, messageIndex, partIndex, msg, eventIDs)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO session_message_parts(session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, ?, ?, ?, ?, ?, ?)`, sessionID, messageIndex, partIndex, part.Kind, part.Text, part.CallKey, eventID); err != nil {
			return err
		}
	}
	return nil
}

func resolveSessionPartEventID(tx sqlWriter, sessionID string, messageIndex int, partIndex int, msg *model.Message, eventIDs map[int]string) (string, error) {
	part := &msg.Parts[partIndex]
	if part.Event == nil {
		return "", nil
	}
	if match := matchingMessageEventIndex(msg.Events, *part.Event); match >= 0 {
		return eventIDs[match], nil
	}
	eventIndex := len(msg.Events) + partIndex
	ensureMessageEventID(sessionID, msg.ID, messageIndex, eventIndex, part.Event)
	if err := insertSessionMessageEventTx(tx, sessionID, messageIndex, eventIndex, *part.Event); err != nil {
		return "", err
	}
	return part.Event.ID, nil
}

func insertSessionMessageEventTx(tx sqlWriter, sessionID string, messageIndex int, eventIndex int, event model.MessageEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = model.NewID()
	}
	meta := strings.TrimSpace(event.Meta)
	if meta == "" && len(event.Details) > 0 {
		meta = compactEventMetaForDB(event.Details)
	}
	if _, err := tx.Exec(`INSERT INTO session_message_events(session_id, message_index, event_index, id, kind, phase, call_key, text, meta) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, sessionID, messageIndex, eventIndex, event.ID, event.Kind, event.Phase, event.CallKey, event.Text, meta); err != nil {
		return err
	}
	if len(event.Details) > 0 {
		detailsRaw, err := json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("encode session %s event %s details: %w", sessionID, event.ID, err)
		}
		if _, err := tx.Exec(`INSERT INTO session_message_event_details(session_id, event_id, details_json, details_bytes, updated_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(session_id, event_id) DO UPDATE SET details_json = excluded.details_json, details_bytes = excluded.details_bytes, updated_at = excluded.updated_at`, sessionID, event.ID, string(detailsRaw), len(detailsRaw), formatScheduleDBTime(time.Now())); err != nil {
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
	row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.session_id = e.session_id AND d.event_id = e.id WHERE e.session_id = ? AND e.id = ?`, sessionID, eventID)
	return scanSessionMessageEventWithDetails(row)
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
		if err := s.db.QueryRow(`SELECT event_id FROM session_message_parts WHERE session_id = ? AND message_index = ? AND part_index = ?`, sessionID, messageIndex, partIndex).Scan(&eventID); err != nil {
			return model.MessageEvent{}, err
		}
		eventIndex = -1
		row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.session_id = e.session_id AND d.event_id = e.id WHERE e.session_id = ? AND e.id = ?`, sessionID, eventID)
		return scanSessionMessageEventWithDetails(row)
	}
	if eventIndex < 0 {
		return model.MessageEvent{}, fmt.Errorf("invalid event index")
	}
	row := s.db.QueryRow(`SELECT e.id, e.kind, e.phase, e.call_key, e.text, e.meta, COALESCE(d.details_json, '') FROM session_message_events e LEFT JOIN session_message_event_details d ON d.session_id = e.session_id AND d.event_id = e.id WHERE e.session_id = ? AND e.message_index = ? AND e.event_index = ?`, sessionID, messageIndex, eventIndex)
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
