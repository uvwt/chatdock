package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
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
	if _, _, _, err := store.PrepareChat("default", model.ChatRequest{SessionID: s.ID, Message: "hello world"}); err != nil {
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
	appendUserMessageForTest(t, store, "default", session.ID, "这是一条可以被会话搜索命中的用户消息")
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
	appendUserMessageForTest(t, store, "default", session.ID, "第一条用户消息")
	if _, err := store.AppendAssistantMessage("default", session.ID, "第一条助手回复"); err != nil {
		t.Fatal(err)
	}
	appendUserMessageForTest(t, store, "default", session.ID, "第二条用户消息")
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
	appendUserMessageForTest(t, store, "default", session.ID, "编辑后的问题")

	prepared, _, history, err := store.PrepareChat("default", model.ChatRequest{SessionID: session.ID, Regenerate: true})
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

func TestAppendUserMessageRollsBackAttachmentBindingWhenSessionSaveFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	attachment := model.AttachmentRecord{
		Attachment: model.Attachment{
			ID:        "attachment-rollback",
			Name:      "rollback.txt",
			MIMEType:  "text/plain",
			Size:      8,
			Status:    "stored",
			CreatedAt: time.Now(),
		},
		StoragePath: "/tmp/rollback.txt",
		SHA256:      "sha-rollback",
	}
	if _, err := store.SaveAttachment(defaultWorkspaceID, attachment); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_session_message_insert
BEFORE INSERT ON session_messages
BEGIN
  SELECT RAISE(ABORT, 'forced session message failure');
END`); err != nil {
		t.Fatal(err)
	}

	if _, err := appendUserMessageWithAttachmentsForTest(store, defaultWorkspaceID, session.ID, "带附件消息", []string{attachment.ID}); err == nil {
		t.Fatal("expected session persistence failure")
	}
	var boundSessionID, boundMessageID string
	if err := store.db.QueryRow(`SELECT session_id, message_id FROM attachments WHERE id = ?`, attachment.ID).Scan(&boundSessionID, &boundMessageID); err != nil {
		t.Fatal(err)
	}
	if boundSessionID != "" || boundMessageID != "" {
		t.Fatalf("attachment binding leaked after rollback: session=%q message=%q", boundSessionID, boundMessageID)
	}
	loaded, ok, err := store.GetSession(defaultWorkspaceID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(loaded.Messages) != 0 {
		t.Fatalf("failed message persisted in session: %#v", loaded)
	}
}

func TestSaveSessionRollsBackWhenWorkspaceTouchFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	session, err := store.CreateSession(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_workspace_touch
BEFORE UPDATE OF updated_at ON workspaces
WHEN OLD.name = 'default'
BEGIN
  SELECT RAISE(ABORT, 'forced workspace touch failure');
END`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AppendAssistantMessage(defaultWorkspaceID, session.ID, "不应持久化"); err == nil {
		t.Fatal("expected workspace touch failure")
	}
	loaded, ok, err := store.GetSession(defaultWorkspaceID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(loaded.Messages) != 0 {
		t.Fatalf("session update survived failed workspace touch: %#v", loaded)
	}
}

func TestPrepareChatJobRollsBackMessageAttachmentAndModelWhenJobInsertFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := model.DefaultModelConfig()
	cfg.BaseURL = "https://example.test/v1"
	cfg.APIKey = "test-secret"
	cfg.Model = "test-model"
	cfg.Models = []string{"test-model"}
	cfg, err = store.SaveModelConfig(defaultWorkspaceID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	attachment := model.AttachmentRecord{
		Attachment:  model.Attachment{ID: "prepare-job-attachment", Name: "job.txt", MIMEType: "text/plain", Size: 3, Status: "stored", CreatedAt: time.Now()},
		StoragePath: "/tmp/job.txt",
		SHA256:      "prepare-job-sha",
	}
	if _, err := store.SaveAttachment(defaultWorkspaceID, attachment); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_chat_job_insert
BEFORE INSERT ON chat_jobs
BEGIN
  SELECT RAISE(ABORT, 'forced chat job insert failure');
END`); err != nil {
		t.Fatal(err)
	}

	_, _, _, _, err = store.PrepareChatJob(defaultWorkspaceID, model.ChatRequest{
		SessionID:     session.ID,
		Message:       "不应部分保存",
		ProviderID:    cfg.ProviderID,
		Model:         cfg.Model,
		AttachmentIDs: []string{attachment.ID},
	}, "req-fail")
	if err == nil {
		t.Fatal("expected chat job insert failure")
	}
	loaded, ok, err := store.GetSession(defaultWorkspaceID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(loaded.Messages) != 0 || loaded.ProviderID != "" || loaded.Model != "" {
		t.Fatalf("session changes survived chat job rollback: %#v", loaded)
	}
	var boundSessionID, boundMessageID string
	if err := store.db.QueryRow(`SELECT session_id, message_id FROM attachments WHERE id = ?`, attachment.ID).Scan(&boundSessionID, &boundMessageID); err != nil {
		t.Fatal(err)
	}
	if boundSessionID != "" || boundMessageID != "" {
		t.Fatalf("attachment binding survived chat job rollback: session=%q message=%q", boundSessionID, boundMessageID)
	}
	jobs, err := store.ListChatJobs(defaultWorkspaceID, session.ID, false, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("failed chat job persisted: %#v", jobs)
	}
}

func TestPrepareChatJobCommitsMessageAttachmentModelAndJob(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := model.DefaultModelConfig()
	cfg.BaseURL = "https://chat-job.example.test/v1"
	cfg.APIKey = "chat-job-secret"
	cfg.Model = "chat-job-model"
	cfg.Models = []string{"chat-job-model"}
	cfg, err = store.SaveModelConfig(defaultWorkspaceID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	attachment := model.AttachmentRecord{
		Attachment:  model.Attachment{ID: "prepare-job-success-file", Name: "success.txt", MIMEType: "text/plain", Size: 7, Status: "stored", CreatedAt: time.Now()},
		StoragePath: "/tmp/prepare-job-success.txt",
		SHA256:      "prepare-job-success-sha",
		TextContent: "附件内容",
	}
	if _, err := store.SaveAttachment(defaultWorkspaceID, attachment); err != nil {
		t.Fatal(err)
	}

	job, savedSession, selected, history, err := store.PrepareChatJob(defaultWorkspaceID, model.ChatRequest{
		SessionID:     session.ID,
		Message:       "请分析附件",
		ProviderID:    cfg.ProviderID,
		Model:         cfg.Model,
		AttachmentIDs: []string{attachment.ID},
	}, "req-success")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" || job.SessionID != session.ID || job.RequestID != "req-success" {
		t.Fatalf("unexpected job: %#v", job)
	}
	if selected.ProviderID != cfg.ProviderID || selected.Model != cfg.Model {
		t.Fatalf("unexpected selected config: %#v", selected)
	}
	if savedSession.ProviderID != cfg.ProviderID || savedSession.Model != cfg.Model || len(savedSession.Messages) != 1 {
		t.Fatalf("unexpected saved session: %#v", savedSession)
	}
	if len(history) != 1 || len(history[0].ModelAttachments) != 1 || history[0].Attachments != nil {
		t.Fatalf("unexpected model history: %#v", history)
	}
	bound, err := store.AttachmentRecordByID(defaultWorkspaceID, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bound.SessionID != session.ID || bound.MessageID != savedSession.Messages[0].ID {
		t.Fatalf("attachment was not bound atomically: %#v", bound)
	}
	storedJob, err := store.GetChatJob(defaultWorkspaceID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedJob.Status != "running" || storedJob.RequestID != "req-success" {
		t.Fatalf("unexpected stored job: %#v", storedJob)
	}
}

func TestPrepareChatClassifiesValidationAndPersistenceErrors(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := store.PrepareChat(defaultWorkspaceID, model.ChatRequest{SessionID: session.ID}); !errors.Is(err, ErrInvalidChatRequest) {
		t.Fatalf("empty message error = %v, want ErrInvalidChatRequest", err)
	}
	if _, _, _, err := store.PrepareChat(defaultWorkspaceID, model.ChatRequest{SessionID: session.ID, Message: "hello", ProviderID: "missing-provider"}); !errors.Is(err, ErrInvalidChatRequest) {
		t.Fatalf("missing provider error = %v, want ErrInvalidChatRequest", err)
	}
	loaded, ok, err := store.GetSession(defaultWorkspaceID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(loaded.Messages) != 0 {
		t.Fatalf("validation failure persisted a message: %#v", loaded)
	}

	if _, err := store.db.Exec(`UPDATE workspace_kv SET value = '{' WHERE workspace_id = ? AND key = 'config'`, defaultWorkspaceID); err != nil {
		t.Fatal(err)
	}
	_, _, _, err = store.PrepareChat(defaultWorkspaceID, model.ChatRequest{SessionID: session.ID, Message: "hello"})
	if err == nil {
		t.Fatal("expected corrupt config error")
	}
	if errors.Is(err, ErrInvalidChatRequest) {
		t.Fatalf("persistence error was misclassified as validation: %v", err)
	}
}
