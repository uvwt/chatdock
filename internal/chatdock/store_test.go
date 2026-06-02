package chatdock

import (
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
