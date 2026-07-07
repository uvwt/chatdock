package store

import (
	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	"testing"
)

func TestStoreMCPRunsAndAgentTasks(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartMCPRun("session-1", "test run")
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"action": "create", "title": "ChatDock MCP"}
	result := map[string]any{"ok": true, "task_id": "task-1", "status": "active"}
	if _, err := store.AddMCPRunEvent(run.ID, RunEventInput{Kind: "tool_call", Status: "success", Tool: "DockMini__task_manage", Arguments: args, Result: result}); err != nil {
		t.Fatal(err)
	}
	finished, err := store.FinishMCPRun(run.ID, "success", "done", nil)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "success" || finished.EventCount != 1 {
		t.Fatalf("unexpected finished run: %#v", finished)
	}
	runs, err := store.ListMCPRuns("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 || runs.Runs[0].ID != run.ID {
		t.Fatalf("unexpected runs: %#v", runs.Runs)
	}
	detail, err := store.MCPRunDetail(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != 1 || detail.Events[0].Tool != "task_manage" || detail.Events[0].Server != "DockMini" {
		t.Fatalf("unexpected run detail: %#v", detail)
	}
	tasks, err := store.ListAgentTasks(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].ID != "task-1" || tasks.Tasks[0].Status != "active" {
		t.Fatalf("unexpected agent tasks: %#v", tasks.Tasks)
	}
}

func TestStoreEffectiveMCPConfigFallsBackToDefaultWorkspace(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defaultConfig := `{"servers":{"agentdock":{"url":"http://host.docker.internal:18766/mcp"}}}`
	if _, err := store.SaveMCPConfig(defaultConfig); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "model", SystemPrompt: "model workspace"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMCPConfig(`{"servers":{}}`); err != nil {
		t.Fatal(err)
	}
	content, err := store.GetEffectiveMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mcp.ParseMCPConfig(content)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Servers["agentdock"]; !ok {
		t.Fatalf("expected default workspace MCP server fallback, got %s", content)
	}
}

func TestStoreChatJobEventsPersistAndFinish(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	job, err := store.CreateChatJob(session.ID, "req_test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddChatJobEvent(job.ID, "delta", llm.StreamDelta{Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	loaded, events, err := store.ChatJobEventsAfter(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "running" || len(events) != 1 || events[0].Event != "delta" || events[0].Seq != 1 {
		t.Fatalf("unexpected loaded job/events: %#v %#v", loaded, events)
	}
	finished, err := store.FinishChatJob(job.ID, "success", "answer", "reason", nil)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "success" || finished.Answer != "answer" || finished.Reasoning != "reason" || finished.FinishedAt == nil {
		t.Fatalf("unexpected finished job: %#v", finished)
	}
}
