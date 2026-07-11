package store

import (
	"strings"
	"testing"
	"time"
)

func TestLoadSessionsRejectsInvalidTableIndexesAndReferences(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	now := formatDBTime(time.Now())

	if _, err := store.db.Exec(`INSERT INTO session_messages(workspace_id, session_id, message_index, id, role, content, reasoning, attachments_json, created_at) VALUES(?, ?, -1, 'negative-message', 'user', '', '', '[]', ?)`, "default", session.ID, now); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "negative message index")
	if _, err := store.db.Exec(`DELETE FROM session_messages WHERE workspace_id = ? AND session_id = ?`, "default", session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`INSERT INTO session_messages(workspace_id, session_id, message_index, id, role, content, reasoning, attachments_json, created_at) VALUES(?, ?, 0, 'message-0', 'user', '', '', '[]', ?)`, "default", session.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO session_message_events(workspace_id, session_id, message_index, event_index, id, kind, phase, call_key, text, meta) VALUES(?, ?, 0, -1, 'negative-event', 'tool', '', '', '', '')`, "default", session.ID); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "invalid indexes")
	if _, err := store.db.Exec(`DELETE FROM session_message_events WHERE workspace_id = ? AND session_id = ?`, "default", session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`INSERT INTO session_message_parts(workspace_id, session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, ?, 0, -1, 'text', '', '', '')`, "default", session.ID); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "invalid indexes")
	if _, err := store.db.Exec(`DELETE FROM session_message_parts WHERE workspace_id = ? AND session_id = ?`, "default", session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`INSERT INTO session_message_parts(workspace_id, session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, ?, 0, 0, 'tool', '', '', 'missing-event')`, "default", session.ID); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "missing event")
}

func assertSessionTableLoadError(t *testing.T, store *Store, expected string) {
	t.Helper()
	_, err := loadSessionsFromTablesLocked(store.db, "default")
	if err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected load error containing %q, got %v", expected, err)
	}
}
