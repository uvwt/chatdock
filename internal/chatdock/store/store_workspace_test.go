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

	if _, err := store.CreatePrompt(model.CreatePromptRequest{Name: "research", SystemPrompt: "研究空间"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveModelConfig(model.ModelConfig{BaseURL: "https://example.test/v1", Model: "demo", SystemPrompt: "研究助手"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMCPConfig(`{"servers":{}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSkill(model.SaveSkillRequest{Name: "研究技能", Content: "只做研究总结。", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateScheduledTask(model.ScheduledTaskRequest{Title: "研究任务", Prompt: "总结研究", Enabled: true, ScheduleType: "interval", IntervalMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(session.ID, "hello research"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SelectPrompt(model.SelectPromptRequest{Name: "default"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeletePrompt(model.SelectPromptRequest{Name: "research"}); err != nil {
		t.Fatal(err)
	}

	assertResearchPromptKVDeleted(t, store)
	assertResearchSessionsDeleted(t, store)
	if _, err := store.WorkspaceConfig("research"); err == nil {
		t.Fatal("deleted workspace config should fail")
	}
	if store.ActivePrompt() != "default" {
		t.Fatalf("active prompt changed after deleting inactive workspace: %s", store.ActivePrompt())
	}
}

func assertResearchPromptKVDeleted(t *testing.T, store *Store) {
	t.Helper()
	var got int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM prompt_kv WHERE prompt = ?", "research").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("prompt_kv rows for deleted workspace = %d, want 0", got)
	}
}

func assertResearchSessionsDeleted(t *testing.T, store *Store) {
	t.Helper()
	var got int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE prompt = ?", "research").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("session rows for deleted workspace = %d, want 0", got)
	}
}
