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
	s, err := store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage("default", s.ID, "hello world"); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.RenameSession("default", s.ID, "new title")
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
	session, err := store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage("default", session.ID, "这是一条可以被会话搜索命中的用户消息"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAssistantMessage("default", session.ID, strings.Repeat("助手总结 ", 30)); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListSessions("default")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
	if summaries[0].Preview == "" || summaries[0].LastRole != "assistant" || len([]rune(summaries[0].Preview)) > 121 {
		t.Fatalf("bad summary preview: %#v", summaries[0])
	}

	cloned, err := store.CloneSession("default", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.ID == session.ID || !strings.Contains(cloned.Title, "副本") || len(cloned.Messages) != 2 {
		t.Fatalf("bad cloned session: %#v", cloned)
	}
	summaries, err = store.ListSessions("default")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("clone should appear in session list: %#v", summaries)
	}
}

func TestStoreBranchSessionCutsAtMessageIndex(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage("default", session.ID, "第一条用户消息"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAssistantMessage("default", session.ID, "第一条助手回复"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage("default", session.ID, "第二条用户消息"); err != nil {
		t.Fatal(err)
	}
	idx := 1
	branched, err := store.BranchSession("default", session.ID, &idx)
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
	session, err := store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateSessionModel("default", session.ID, " provider-a ", " model-x ")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ProviderID != "provider-a" || updated.Model != "model-x" {
		t.Fatalf("unexpected model selection: %#v", updated)
	}
	loaded, ok, err := store.GetSession("default", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("session missing")
	}
	if loaded.ProviderID != "provider-a" || loaded.Model != "model-x" {
		t.Fatalf("model selection was not persisted in session: %#v", loaded)
	}
	summaries, err := store.ListSessions("default")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ProviderID != "provider-a" || summaries[0].Model != "model-x" {
		t.Fatalf("model selection missing from summaries: %#v", summaries)
	}
}

func TestStorePrepareSessionRegenerationUsesLastUserWithoutAppending(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage("default", session.ID, "编辑后的问题"); err != nil {
		t.Fatal(err)
	}

	prepared, _, history, err := store.PrepareSessionRegeneration("default", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Messages) != 1 || len(history) != 1 {
		t.Fatalf("regeneration should not append a user message: prepared=%d history=%d", len(prepared.Messages), len(history))
	}
	if history[0].Role != "user" || history[0].Content != "编辑后的问题" {
		t.Fatalf("unexpected regeneration history: %#v", history)
	}
	loaded, ok, err := store.GetSession("default", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("session missing")
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("store message count changed during regeneration prep: %#v", loaded.Messages)
	}
}
