package store

import (
	"strings"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestStoreDeleteWorkspaceCascadesPromptData(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "research", SystemPrompt: "研究空间"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveModelConfig("research", model.ModelConfig{BaseURL: "https://example.test/v1", Model: "demo", SystemPrompt: "研究助手"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMCPConfig("research", `{"servers":{}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateScheduledTask("research", model.ScheduledTaskRequest{Title: "研究任务", Prompt: "总结研究", Enabled: true, ScheduleType: "interval", IntervalMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("research")
	if err != nil {
		t.Fatal(err)
	}
	appendUserMessageForTest(t, store, "research", session.ID, "hello research")

	response, err := store.DeleteWorkspace(model.WorkspaceIDRequest{Name: "research"})
	if err != nil {
		t.Fatal(err)
	}

	assertResearchPromptKVDeleted(t, store)
	assertResearchSessionsDeleted(t, store)
	if _, err := store.WorkspaceConfig("research"); err == nil {
		t.Fatal("deleted workspace config should fail")
	}
	if response.Active != defaultWorkspaceID {
		t.Fatalf("active workspace = %q, want %q", response.Active, defaultWorkspaceID)
	}
}

func assertResearchPromptKVDeleted(t *testing.T, store *Store) {
	t.Helper()
	var got int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM workspace_kv WHERE workspace_id = ?", "research").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("workspace_kv rows for deleted workspace = %d, want 0", got)
	}
}

func assertResearchSessionsDeleted(t *testing.T, store *Store) {
	t.Helper()
	var got int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE workspace_id = ?", "research").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("session rows for deleted workspace = %d, want 0", got)
	}
}

func TestResolveChatModelConfigUsesSelectedWorkspaceProvider(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveModelConfig("default", model.ModelConfig{BaseURL: "https://default.test/v1", Model: "default-model", Models: []string{"default-model"}, SystemPrompt: "当前空间提示词"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "alt", SystemPrompt: "另一个空间"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWorkspaceConfig("alt", model.ModelConfig{BaseURL: "https://alt.test/v1", Model: "alt-a", Models: []string{"alt-a", "alt-b"}, APIKey: "alt-key", SystemPrompt: "另一个空间提示词"}); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	_, selected, _, err := store.PrepareChat("default", model.ChatRequest{SessionID: session.ID, Message: "选择另一个供应商", ProviderID: "alt", Model: "alt-b"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.BaseURL != "https://alt.test/v1" || selected.APIKey != "alt-key" || selected.Model != "alt-b" {
		t.Fatalf("provider/model not selected: %#v", selected)
	}
	if selected.SystemPrompt != "当前空间提示词" {
		t.Fatalf("chat prompt should stay on current workspace: %q", selected.SystemPrompt)
	}
}

func TestCreateWorkspaceRollsBackWhenMCPInitializationFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	if _, err := store.db.Exec(`CREATE TRIGGER fail_workspace_mcp
BEFORE INSERT ON workspace_kv
WHEN NEW.workspace_id = 'broken' AND NEW.key = 'mcp'
BEGIN
  SELECT RAISE(ABORT, 'forced mcp failure');
END`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "broken", SystemPrompt: "不应保留"}); err == nil {
		t.Fatal("expected workspace initialization error")
	}
	assertWorkspaceAbsent(t, store, "broken")
}

func TestCreateWorkspaceRejectsCorruptDefaultConfig(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	if _, err := store.db.Exec(`UPDATE workspace_kv SET value = '{' WHERE workspace_id = ? AND key = 'config'`, defaultWorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "broken-config"}); err == nil {
		t.Fatal("expected corrupt default config error")
	}
	assertWorkspaceAbsent(t, store, "broken-config")
}

func assertWorkspaceAbsent(t *testing.T, store *Store, name string) {
	t.Helper()
	var workspaceCount, kvCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE name = ?`, name).Scan(&workspaceCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM workspace_kv WHERE workspace_id = ?`, name).Scan(&kvCount); err != nil {
		t.Fatal(err)
	}
	if workspaceCount != 0 || kvCount != 0 {
		t.Fatalf("partial workspace remains: workspaces=%d workspace_kv=%d", workspaceCount, kvCount)
	}
}

func TestDeleteWorkspaceRejectsRunningChatJob(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "research"}); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("research")
	if err != nil {
		t.Fatal(err)
	}
	job, err := createChatJobForTest(t, store, "research", session.ID, "req-running")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.DeleteWorkspace(model.WorkspaceIDRequest{Name: "research"}); err == nil || !strings.Contains(err.Error(), "chat_jobs=1") {
		t.Fatalf("expected running chat job guard, got %v", err)
	}
	if _, err := store.InterruptChatJob("research", job.ID, "test cleanup"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteWorkspace(model.WorkspaceIDRequest{Name: "research"}); err != nil {
		t.Fatalf("delete after job interruption: %v", err)
	}
}

func TestDeleteWorkspaceRejectsRunningScheduledTask(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "automation"}); err != nil {
		t.Fatal(err)
	}
	response, err := store.CreateScheduledTask("automation", model.ScheduledTaskRequest{Title: "运行中任务", Prompt: "执行", Enabled: true, ScheduleType: "interval", IntervalMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Tasks) != 1 {
		t.Fatalf("unexpected tasks: %#v", response.Tasks)
	}
	if _, err := store.PrepareScheduledTaskRunInWorkspace("automation", response.Tasks[0].ID, true, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := store.DeleteWorkspace(model.WorkspaceIDRequest{Name: "automation"}); err == nil || !strings.Contains(err.Error(), "scheduled_tasks=1") {
		t.Fatalf("expected running scheduled task guard, got %v", err)
	}
}

func TestListWorkspacesPropagatesScheduledTaskReadFailure(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	if _, err := store.db.Exec(`DROP TABLE scheduled_tasks`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListWorkspaces(defaultWorkspaceID); err == nil {
		t.Fatal("workspace list must not hide scheduled task read failures")
	}
}

func TestModelConfigReturnsCorruptWorkspaceConfigError(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`UPDATE workspace_kv SET value = '{' WHERE workspace_id = ? AND key = 'config'`, defaultWorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ModelConfig(defaultWorkspaceID); err == nil {
		t.Fatal("corrupt workspace config must not return a default config")
	}
}

func TestSaveModelConfigRollsBackProviderConfigAndEmbeddings(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	initial := model.DefaultModelConfig()
	initial.BaseURL = "https://old.example.test/v1"
	initial.APIKey = "old-secret"
	initial.Model = "old-model"
	initial.Models = []string{"old-model"}
	initial.EmbeddingBaseURL = "https://embedding-old.example.test/v1"
	initial.EmbeddingModel = "embed-old"
	saved, err := store.SaveModelConfig(defaultWorkspaceID, initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveToolEmbedding(defaultWorkspaceID, ToolEmbeddingRecord{FullName: "demo_tool", SourceHash: "hash-old", EmbeddingModel: saved.EmbeddingModel, Embedding: []float64{0.1, 0.2}}); err != nil {
		t.Fatal(err)
	}
	beforeMeta, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	beforeConfig, ok, err := store.getWorkspaceRawLocked(defaultWorkspaceID, "config")
	if err != nil || !ok {
		t.Fatalf("read initial config: ok=%v err=%v", ok, err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_model_config_write
BEFORE UPDATE ON workspace_kv
WHEN OLD.workspace_id = 'default' AND OLD.key = 'config'
BEGIN
  SELECT RAISE(ABORT, 'forced model config failure');
END`); err != nil {
		t.Fatal(err)
	}

	next := saved
	next.BaseURL = "https://new.example.test/v1"
	next.APIKey = "new-secret"
	next.Model = "new-model"
	next.Models = []string{"new-model"}
	next.EmbeddingBaseURL = "https://embedding-new.example.test/v1"
	next.EmbeddingModel = "embed-new"
	if _, err := store.SaveModelConfig(defaultWorkspaceID, next); err == nil {
		t.Fatal("expected model config write failure")
	}

	afterMeta, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	afterConfig, ok, err := store.getWorkspaceRawLocked(defaultWorkspaceID, "config")
	if err != nil || !ok {
		t.Fatalf("read config after rollback: ok=%v err=%v", ok, err)
	}
	if afterMeta != beforeMeta {
		t.Fatal("provider metadata changed despite config transaction rollback")
	}
	if afterConfig != beforeConfig {
		t.Fatal("workspace config changed despite transaction rollback")
	}
	embeddings, err := store.ToolEmbeddings(defaultWorkspaceID, "embed-old")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := embeddings["demo_tool"]; !ok {
		t.Fatal("tool embedding was deleted despite config transaction rollback")
	}
}

func TestEnsureWorkspaceDefaultsRollsBackPartialInitialization(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	if _, err := store.db.Exec(`INSERT INTO workspaces(name, created_at, updated_at) VALUES(?, ?, ?)`, "partial", formatDBTime(now), formatDBTime(now)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_default_mcp_write
BEFORE INSERT ON workspace_kv
WHEN NEW.workspace_id = 'partial' AND NEW.key = 'mcp'
BEGIN
  SELECT RAISE(ABORT, 'forced default mcp failure');
END`); err != nil {
		t.Fatal(err)
	}

	if err := store.ensureWorkspaceDefaultsLocked("partial"); err == nil {
		t.Fatal("expected default initialization failure")
	}
	var kvCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM workspace_kv WHERE workspace_id = 'partial'`).Scan(&kvCount); err != nil {
		t.Fatal(err)
	}
	if kvCount != 0 {
		t.Fatalf("partial workspace defaults persisted: %d", kvCount)
	}
}

func TestWorkspaceListsResolveMissingActiveWorkspace(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "research"}); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListWorkspaceSummaries("deleted-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if summaries.Active != defaultWorkspaceID {
		t.Fatalf("summary active = %q, want %q", summaries.Active, defaultWorkspaceID)
	}
	activeCount := 0
	for _, item := range summaries.Workspaces {
		if item.Active {
			activeCount++
			if item.Name != defaultWorkspaceID {
				t.Fatalf("unexpected active summary: %#v", item)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("active summary count = %d, want 1", activeCount)
	}

	full, err := store.ListWorkspaces("deleted-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if full.Active != defaultWorkspaceID {
		t.Fatalf("workspace active = %q, want %q", full.Active, defaultWorkspaceID)
	}
	activeCount = 0
	for _, item := range full.Workspaces {
		if item.Active {
			activeCount++
			if item.ID != defaultWorkspaceID {
				t.Fatalf("unexpected active workspace: %#v", item)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("active workspace count = %d, want 1", activeCount)
	}
}

func TestDeleteWorkspaceRecalculatesAttachmentBlobReferences(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "files"}); err != nil {
		t.Fatal(err)
	}
	record := model.AttachmentRecord{
		Attachment:  model.Attachment{ID: "workspace-file", Name: "file.txt", MIMEType: "text/plain", Size: 4, Status: "stored", CreatedAt: time.Now()},
		StoragePath: "/tmp/workspace-file.txt",
		SHA256:      "workspace-file-sha",
	}
	if _, err := store.SaveAttachment("files", record); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteWorkspace(model.WorkspaceIDRequest{Name: "files"}); err != nil {
		t.Fatal(err)
	}
	blob, ok, err := store.AttachmentBlobBySHA256(record.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || blob.RefCount != 0 {
		t.Fatalf("blob refcount after workspace delete: %#v", blob)
	}
}
