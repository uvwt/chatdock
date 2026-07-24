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
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	now := formatDBTime(time.Now())

	if _, err := store.db.Exec(`INSERT INTO session_messages(session_id, message_index, id, role, content, reasoning, attachments_json, created_at) VALUES(?, -1, 'negative-message', 'user', '', '', '[]', ?)`, session.ID, now); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "negative message index")
	if _, err := store.db.Exec(`DELETE FROM session_messages WHERE session_id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`INSERT INTO session_messages(session_id, message_index, id, role, content, reasoning, attachments_json, created_at) VALUES(?, 0, 'message-0', 'user', '', '', '[]', ?)`, session.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO session_message_events(session_id, message_index, event_index, id, kind, phase, call_key, text, meta) VALUES(?, 0, -1, 'negative-event', 'tool', '', '', '', '')`, session.ID); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "invalid indexes")
	if _, err := store.db.Exec(`DELETE FROM session_message_events WHERE session_id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`INSERT INTO session_message_parts(session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, 0, -1, 'text', '', '', '')`, session.ID); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "invalid indexes")
	if _, err := store.db.Exec(`DELETE FROM session_message_parts WHERE session_id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.Exec(`INSERT INTO session_message_parts(session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, 0, 0, 'tool', '', '', 'missing-event')`, session.ID); err != nil {
		t.Fatal(err)
	}
	assertSessionTableLoadError(t, store, "missing event")
}

func assertSessionTableLoadError(t *testing.T, store *Store, expected string) {
	t.Helper()
	_, err := loadSessionsFromTablesLocked(store.db)
	if err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected load error containing %q, got %v", expected, err)
	}
}
