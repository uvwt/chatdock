package store

import (
	"chatdock/internal/chatdock/model"
	"testing"
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
	if _, _, _, err := store.AppendUserMessage("research", session.ID, "hello research"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.DeleteWorkspace(model.WorkspaceIDRequest{Name: "research"}); err != nil {
		t.Fatal(err)
	}

	assertResearchPromptKVDeleted(t, store)
	assertResearchSessionsDeleted(t, store)
	if _, err := store.WorkspaceConfig("research"); err == nil {
		t.Fatal("deleted workspace config should fail")
	}
	if "default" != "default" {
		t.Fatalf("active prompt changed after deleting inactive workspace: %s", "default")
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
	base := store.GetModelConfig("default")
	selected, err := store.ResolveChatModelConfig(base, "alt", "alt-b")
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
