package store

import (
	"testing"

	"chatdock/internal/model"
)

func TestSessionFreezesGlobalAndProjectPromptsIncludingEmptyProjectPrompt(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.SaveModelConfig(model.ModelConfig{BaseURL: "https://provider.example/v1", Model: "demo", SystemPrompt: "全局 v1"}); err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(model.CreateProjectRequest{Name: "项目", Prompt: "项目 v1"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !session.SystemPromptFrozen || !session.ProjectPromptFrozen || session.SystemPromptSnapshot != "全局 v1" || session.ProjectPromptSnapshot != "项目 v1" {
		t.Fatalf("prompt snapshots = %#v", session)
	}

	if _, err := store.SaveModelConfig(model.ModelConfig{BaseURL: "https://provider.example/v1", Model: "demo", SystemPrompt: "全局 v2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(project.ID, model.UpdateProjectRequest{Name: project.Name, Prompt: "项目 v2"}); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := store.SessionProviderContext(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SystemPrompt != "全局 v1\n\n项目 v1" {
		t.Fatalf("frozen provider prompt = %q", cfg.SystemPrompt)
	}

	emptyProject, err := store.CreateProject(model.CreateProjectRequest{Name: "空项目", Prompt: ""})
	if err != nil {
		t.Fatal(err)
	}
	emptySession, err := store.CreateSession(emptyProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !emptySession.ProjectPromptFrozen || emptySession.ProjectPromptSnapshot != "" {
		t.Fatalf("empty project prompt was not frozen: %#v", emptySession)
	}
	if _, err := store.UpdateProject(emptyProject.ID, model.UpdateProjectRequest{Name: emptyProject.Name, Prompt: "后来增加的项目提示词"}); err != nil {
		t.Fatal(err)
	}
	emptyCfg, _, err := store.SessionProviderContext(emptySession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if emptyCfg.SystemPrompt != "全局 v2" {
		t.Fatalf("empty project changed frozen prompt = %q", emptyCfg.SystemPrompt)
	}
}

func TestContextCheckpointsCloneButBranchAndEditClear(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	prepared := appendUserMessageForTest(t, store, session.ID, "原始问题")
	loaded, ok, err := store.GetSession(session.ID)
	if err != nil || !ok || len(loaded.Messages) != 1 {
		t.Fatalf("load session: ok=%v err=%v session=%#v", ok, err, loaded)
	}
	if err := store.SaveContextCheckpoint(ContextCheckpoint{SessionID: session.ID, ProviderID: "provider-a", Model: "model-a", Summary: "早期摘要", CutoffMessageID: prepared.Messages[0].ID, CutoffMessageIndex: 0}); err != nil {
		t.Fatal(err)
	}

	cloned, err := store.CloneSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetContextCheckpoint(cloned.ID, "provider-a", "model-a"); err != nil || !ok {
		t.Fatalf("clone checkpoint: ok=%v err=%v", ok, err)
	}
	branched, err := store.BranchSession(session.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetContextCheckpoint(branched.ID, "provider-a", "model-a"); err != nil || ok {
		t.Fatalf("branch checkpoint: ok=%v err=%v", ok, err)
	}
	if _, err := store.EditUserMessageAndTruncate(session.ID, loaded.Messages[0].ID, nil, "编辑后的问题"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetContextCheckpoint(session.ID, "provider-a", "model-a"); err != nil || ok {
		t.Fatalf("edited checkpoint: ok=%v err=%v", ok, err)
	}
}

func TestAssistantUsageIsPersistedAndSummarized(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	usage := &model.Usage{InputTokens: 100, OutputTokens: 20, ReasoningTokens: 5, CacheHitTokens: 60, CacheMissTokens: 40, TotalTokens: 125, Source: "provider"}
	if _, err := store.AppendAssistantMessageWithReasoningAndUsage(session.ID, "回答", "推理", usage); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.GetSession(session.ID)
	if err != nil || !ok {
		t.Fatalf("load session: ok=%v err=%v", ok, err)
	}
	last := loaded.Messages[len(loaded.Messages)-1]
	if last.Usage == nil || last.Usage.TotalTokens != 125 || loaded.UsageSummary == nil || loaded.UsageSummary.ReplyCount != 1 || loaded.UsageSummary.CacheHitTokens != 60 {
		t.Fatalf("usage persistence = message:%#v summary:%#v", last.Usage, loaded.UsageSummary)
	}
}
