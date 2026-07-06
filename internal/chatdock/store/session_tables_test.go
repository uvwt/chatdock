package store

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestSessionJSONMigratesToTablesAndPersistsCompactLegacyRow(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.mu.Lock()
	if _, err := st.db.Exec(`DELETE FROM meta WHERE key = ?`, sessionTablesMigratedKey); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 6, 9, 30, 0, 0, time.UTC)
	legacy := model.Session{ID: "session-legacy", Title: "旧会话", CreatedAt: now, UpdatedAt: now, Messages: []model.Message{{ID: "m1", Role: "assistant", Content: "answer", CreatedAt: now, Events: []model.MessageEvent{{Kind: "tool", Phase: "done", Text: "tool done", Details: map[string]any{"result": map[string]any{"value": "large detail"}}}}, Parts: []model.MessagePart{{Kind: "event", Text: "tool done", Event: &model.MessageEvent{Kind: "tool", Phase: "done", Text: "tool done", Details: map[string]any{"result": map[string]any{"value": "large detail"}}}}}}}}
	raw, _ := json.Marshal(legacy)
	if _, err := st.db.Exec(`INSERT INTO sessions(prompt, id, json, created_at, updated_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(prompt, id) DO UPDATE SET json = excluded.json, updated_at = excluded.updated_at`, defaultPromptName, legacy.ID, string(raw), formatScheduleDBTime(now), formatScheduleDBTime(now)); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	if err := st.migrateSessionJSONToTables(); err != nil {
		t.Fatal(err)
	}
	var msgRows, eventRows, detailRows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM session_messages WHERE session_id = ?`, legacy.ID).Scan(&msgRows); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM session_message_events WHERE session_id = ?`, legacy.ID).Scan(&eventRows); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM session_message_event_details WHERE session_id = ?`, legacy.ID).Scan(&detailRows); err != nil {
		t.Fatal(err)
	}
	if msgRows != 1 || eventRows < 1 || detailRows < 1 {
		t.Fatalf("migrated rows msg=%d events=%d details=%d", msgRows, eventRows, detailRows)
	}

	if err := st.loadPromptLocked(defaultPromptName); err != nil {
		t.Fatal(err)
	}
	session, ok := st.GetSession(legacy.ID)
	if !ok {
		t.Fatal("migrated session not loaded")
	}
	if len(session.Messages) != 1 || len(session.Messages[0].Events) == 0 {
		t.Fatalf("session did not reconstruct event summary: %#v", session)
	}
	if session.Messages[0].Events[0].Details != nil {
		t.Fatalf("event details should be lazy-loaded, got %#v", session.Messages[0].Events[0].Details)
	}
	if len(session.Messages[0].Parts) != 1 || session.Messages[0].Parts[0].Event == nil {
		t.Fatalf("session did not reconstruct part event: %#v", session.Messages[0].Parts)
	}
	fullEvent, err := st.SessionMessageEventByID(legacy.ID, session.Messages[0].Events[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if fullEvent.Details["result"] == nil {
		t.Fatalf("lazy event detail not loaded: %#v", fullEvent)
	}

	if _, err := st.AppendAssistantMessage(legacy.ID, "new answer"); err != nil {
		t.Fatal(err)
	}
	var compactRaw string
	if err := st.db.QueryRow(`SELECT json FROM sessions WHERE id = ?`, legacy.ID).Scan(&compactRaw); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	var compact model.Session
	if err := json.Unmarshal([]byte(compactRaw), &compact); err != nil {
		t.Fatal(err)
	}
	if len(compact.Messages) != 0 {
		t.Fatalf("legacy sessions.json should be compact metadata only, got %d messages", len(compact.Messages))
	}
}
