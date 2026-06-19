package chatdock

import (
	"strings"
	"testing"
	"time"
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
	md := sessionToMarkdown(renamed)
	if !strings.Contains(md, "hello world") || !strings.Contains(md, "new title") {
		t.Fatalf("bad markdown export: %s", md)
	}
}

func TestStoreSkillsInjectedIntoChatConfig(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSkill(SaveSkillRequest{Name: "代码审查", Description: "审查 Go 代码", Content: "指出风险并给出修改建议。", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Skills) != 1 || !created.Skills[0].Enabled {
		t.Fatalf("unexpected skills: %#v", created.Skills)
	}
	s, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	_, cfg, _, err := store.AppendUserMessage(s.ID, "检查这段代码")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0].Name != "代码审查" {
		t.Fatalf("skill not injected into config: %#v", cfg.Skills)
	}
	prompt := buildSystemPrompt(cfg)
	if !strings.Contains(prompt, "# 已启用技能") || !strings.Contains(prompt, "指出风险并给出修改建议") {
		t.Fatalf("skill not injected into system prompt: %s", prompt)
	}
}

func TestStoreScheduledTaskLifecycle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runAt := time.Now().Add(time.Hour).Format("2006-01-02T15:04")
	created, err := store.CreateScheduledTask(ScheduledTaskRequest{Title: "日报", Prompt: "总结今天", Enabled: true, ScheduleType: "once", RunAt: runAt})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 || created.Tasks[0].ID == "" || created.Tasks[0].NextRunAt.IsZero() {
		t.Fatalf("unexpected created tasks: %#v", created.Tasks)
	}

	updated, err := store.UpdateScheduledTask(created.Tasks[0].ID, ScheduledTaskRequest{Title: "日报", Prompt: "总结今天", Enabled: true, ScheduleType: "interval", IntervalMinutes: 5})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Tasks[0].ScheduleType != "interval" || updated.Tasks[0].IntervalMinutes != 5 {
		t.Fatalf("unexpected updated tasks: %#v", updated.Tasks)
	}

	deleted, err := store.DeleteScheduledTask(created.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Tasks) != 0 {
		t.Fatalf("unexpected deleted tasks: %#v", deleted.Tasks)
	}
}

func TestStoreDeleteWorkspaceCascadesPromptData(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreatePrompt(CreatePromptRequest{Name: "research", SystemPrompt: "研究空间"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveModelConfig(ModelConfig{BaseURL: "https://example.test/v1", Model: "demo", SystemPrompt: "研究助手"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMCPConfig(`{"servers":{}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSkill(SaveSkillRequest{Name: "研究技能", Content: "只做研究总结。", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateScheduledTask(ScheduledTaskRequest{Title: "研究任务", Prompt: "总结研究", Enabled: true, ScheduleType: "interval", IntervalMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(session.ID, "hello research"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SelectPrompt(SelectPromptRequest{Name: "default"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeletePrompt(SelectPromptRequest{Name: "research"}); err != nil {
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
