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

func TestStoreBranchSessionCutsAtMessageIndex(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(session.ID, "第一条用户消息"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAssistantMessage(session.ID, "第一条助手回复"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(session.ID, "第二条用户消息"); err != nil {
		t.Fatal(err)
	}
	idx := 1
	branched, err := store.BranchSession(session.ID, &idx)
	if err != nil {
		t.Fatal(err)
	}
	if branched.ID == session.ID || !strings.Contains(branched.Title, "分支") || len(branched.Messages) != 2 {
		t.Fatalf("bad branched session: %#v", branched)
	}
	if branched.Messages[1].Content != "第一条助手回复" {
		t.Fatalf("branch should keep messages through index 1: %#v", branched.Messages)
	}
}

func TestStoreUpdateSessionModelPersistsAndAppearsInSummary(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateSessionModel(session.ID, " provider-a ", " model-x ")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ProviderID != "provider-a" || updated.Model != "model-x" {
		t.Fatalf("unexpected model selection: %#v", updated)
	}
	loaded, ok := store.GetSession(session.ID)
	if !ok {
		t.Fatal("session missing")
	}
	if loaded.ProviderID != "provider-a" || loaded.Model != "model-x" {
		t.Fatalf("model selection was not persisted in session: %#v", loaded)
	}
	summaries := store.ListSessions()
	if len(summaries) != 1 || summaries[0].ProviderID != "provider-a" || summaries[0].Model != "model-x" {
		t.Fatalf("model selection missing from summaries: %#v", summaries)
	}
}
