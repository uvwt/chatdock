package store

import (
	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
	"strings"
	"testing"
	"time"
)

func TestStoreSkillsInjectedIntoChatConfig(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSkill(model.SaveSkillRequest{Name: "代码审查", Description: "审查 Go 代码", Content: "指出风险并给出修改建议。", Enabled: true})
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
	prompt := llm.BuildSystemPrompt(cfg)
	if !strings.Contains(prompt, "# 已启用技能") || !strings.Contains(prompt, "指出风险并给出修改建议") {
		t.Fatalf("skill not injected into system prompt: %s", prompt)
	}
}

func TestStorePinSessionSortsBeforeRecentUnpinned(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	older, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	newer, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PinSession(older.ID, true); err != nil {
		t.Fatal(err)
	}
	summaries := store.ListSessions()
	if len(summaries) != 2 || summaries[0].ID != older.ID || !summaries[0].Pinned || summaries[1].ID != newer.ID {
		t.Fatalf("pinned session should sort first: %#v", summaries)
	}
	if _, err := store.PinSession(older.ID, false); err != nil {
		t.Fatal(err)
	}
	summaries = store.ListSessions()
	if summaries[0].ID != newer.ID || summaries[0].Pinned {
		t.Fatalf("unpinned sessions should sort by updated_at: %#v", summaries)
	}
}

func TestStoreScheduledTaskLifecycle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runAt := time.Now().Add(time.Hour).Format("2006-01-02T15:04")
	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{Title: "日报", Prompt: "总结今天", Enabled: true, ScheduleType: "once", RunAt: runAt})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 || created.Tasks[0].ID == "" || created.Tasks[0].NextRunAt.IsZero() {
		t.Fatalf("unexpected created tasks: %#v", created.Tasks)
	}

	updated, err := store.UpdateScheduledTask(created.Tasks[0].ID, model.ScheduledTaskRequest{Title: "日报", Prompt: "总结今天", Enabled: true, ScheduleType: "interval", IntervalMinutes: 5})
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
