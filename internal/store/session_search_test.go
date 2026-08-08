package store

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSearchSessionsFindsAttachmentTextInOneQuery(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	first, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id        string
		sessionID string
		filename  string
		text      string
	}{
		{id: "attachment-first", sessionID: first.ID, filename: "roadmap.txt", text: "青山计划第一阶段"},
		{id: "attachment-second", sessionID: second.ID, filename: "notes.txt", text: "普通会议记录"},
	} {
		if _, err := store.db.Exec(`INSERT INTO attachments(id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at) VALUES(?, ?, '', ?, 'text/plain', 1, ?, ?, ?, 'stored', ?)`, row.id, row.sessionID, row.filename, row.id+".txt", row.id+"-sha", row.text, time.Now().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.SearchSessions("青山计划", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != first.ID {
		t.Fatalf("unexpected attachment search results: %#v", results)
	}
	if results[0].MatchField != "附件文本" || !strings.Contains(results[0].MatchSnippet, "青山计划") {
		t.Fatalf("unexpected attachment match: %#v", results[0])
	}
}

func TestSearchSessionsReturnsAttachmentQueryErrors(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CreateSession(""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE attachments`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SearchSessions("anything", 20); err == nil {
		t.Fatal("attachment query failure should not be treated as an empty search result")
	}
}

func TestMatchSessionTextKeepsUnicodeSnippetPositionsRuneSafe(t *testing.T) {
	text := "İ" + strings.Repeat("a", 28) + "s-end"
	score, field, snippet := matchSessionText(text, "标题", "s", "s")
	if score <= 0 || field != "标题" {
		t.Fatalf("unexpected match: score=%d field=%q snippet=%q", score, field, snippet)
	}
	if !utf8.ValidString(snippet) {
		t.Fatalf("snippet is not valid UTF-8: %q", snippet)
	}
	if !strings.HasPrefix(snippet, "…") || strings.Contains(snippet, "İ") || !strings.Contains(snippet, "s-end") {
		t.Fatalf("snippet used a lowercase byte offset instead of the original rune position: %q", snippet)
	}
}
