package store

import (
	"chatdock/internal/chatdock/model"
	"strings"
	"testing"
)

func TestStoreSessionRenameAndExport(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(s.ID, "hello world"); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.RenameSession(s.ID, "new title")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "new title" {
		t.Fatalf("unexpected title: %s", renamed.Title)
	}
	md := sessionToMarkdownForTest(renamed)
	if !strings.Contains(md, "hello world") || !strings.Contains(md, "new title") {
		t.Fatalf("bad markdown export: %s", md)
	}
}

func sessionToMarkdownForTest(session *model.Session) string {
	var b strings.Builder
	b.WriteString("# " + session.Title + "\n\n")
	for _, msg := range session.Messages {
		b.WriteString("## " + msg.Role + "\n\n")
		b.WriteString(msg.Content + "\n\n")
	}
	return b.String()
}

func TestStoreSessionSummaryPreviewAndClone(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(session.ID, "这是一条可以被会话搜索命中的用户消息"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAssistantMessage(session.ID, strings.Repeat("助手总结 ", 30)); err != nil {
		t.Fatal(err)
	}

	summaries := store.ListSessions()
	if len(summaries) != 1 {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
	if summaries[0].Preview == "" || summaries[0].LastRole != "assistant" || len([]rune(summaries[0].Preview)) > 121 {
		t.Fatalf("bad summary preview: %#v", summaries[0])
	}

	cloned, err := store.CloneSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.ID == session.ID || !strings.Contains(cloned.Title, "副本") || len(cloned.Messages) != 2 {
		t.Fatalf("bad cloned session: %#v", cloned)
	}
	summaries = store.ListSessions()
	if len(summaries) != 2 {
		t.Fatalf("clone should appear in session list: %#v", summaries)
	}
}
