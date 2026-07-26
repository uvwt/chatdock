package store

import (
	"testing"

	"chatdock/internal/model"
)

func TestSessionProviderContextUsesCurrentChatConfigurationWithoutMutatingSession(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.SaveModelConfig(model.ModelConfig{
		BaseURL:      "https://provider.example/v1",
		Model:        "demo-model",
		SystemPrompt: "全局提示词",
		ContextMode:  model.ContextModeAuto,
	}); err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(model.CreateProjectRequest{Name: "研究", Prompt: "项目提示词"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	appendUserMessageForTest(t, store, session.ID, "第一条消息")

	cfg, history, err := store.SessionProviderContext(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SystemPrompt != "全局提示词\n\n项目提示词" {
		t.Fatalf("system prompt = %q", cfg.SystemPrompt)
	}
	if cfg.Model != "demo-model" || len(history) != 1 || history[0].Content != "第一条消息" {
		t.Fatalf("provider context = cfg:%#v history:%#v", cfg, history)
	}

	history[0].Content = "被调用方修改"
	persisted, ok, err := store.GetSession(session.ID)
	if err != nil || !ok {
		t.Fatalf("load session: ok=%v err=%v", ok, err)
	}
	if persisted.Messages[0].Content != "第一条消息" {
		t.Fatalf("provider context leaked mutation into session: %#v", persisted.Messages)
	}
}
