package store

import (
	"testing"
	"time"

	"chatdock/internal/llm"
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

func TestStoreChatJobEventsPersistAndFinish(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	job, err := createChatJobForTest(t, store, session.ID, "req_test")
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
	_, events, err = store.ChatJobEventsAfter(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "delta" {
		t.Fatalf("finished job must retain streaming deltas until delayed cleanup: %#v", events)
	}
	if _, err := store.FinishChatJob("missing", "success", "", "", nil); err == nil {
		t.Fatal("missing chat job must not finish successfully")
	}
}

func TestPruneChatJobStreamingEventsKeepsRecentAndNonStreamingEvents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	oldSession, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	oldJob, err := createChatJobForTest(t, store, oldSession.ID, "req_old")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddChatJobEvent(oldJob.ID, "delta", llm.StreamDelta{Content: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddChatJobEvent(oldJob.ID, "tool_setup_ready", map[string]any{"tool_count": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishChatJob(oldJob.ID, "success", "old", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE chat_jobs SET finished_at = ? WHERE id = ?`, formatDBTime(time.Now().Add(-48*time.Hour)), oldJob.ID); err != nil {
		t.Fatal(err)
	}

	recentSession, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	recentJob, err := createChatJobForTest(t, store, recentSession.ID, "req_recent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddChatJobEvent(recentJob.ID, "delta", llm.StreamDelta{Content: "recent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishChatJob(recentJob.ID, "success", "recent", "", nil); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.PruneChatJobStreamingEventsBefore(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected one expired delta deleted, got %d", deleted)
	}
	_, oldEvents, err := store.ChatJobEventsAfter(oldJob.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldEvents) != 1 || oldEvents[0].Event != "tool_setup_ready" {
		t.Fatalf("non-streaming events must survive cleanup: %#v", oldEvents)
	}
	_, recentEvents, err := store.ChatJobEventsAfter(recentJob.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recentEvents) != 1 || recentEvents[0].Event != "delta" {
		t.Fatalf("recent streaming events must survive cleanup: %#v", recentEvents)
	}
}

func TestInterruptChatJobPersistsCancellationEvent(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	job, err := createChatJobForTest(t, store, session.ID, "req_cancel")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.InterruptChatJob(job.ID, "stop now")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "interrupted" || cancelled.Error != "stop now" {
		t.Fatalf("unexpected interrupted job: %#v", cancelled)
	}
	_, events, err := store.ChatJobEventsAfter(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "job_cancelled" || events[0].Seq != 1 {
		t.Fatalf("unexpected cancellation events: %#v", events)
	}
}

func TestAddMCPRunEventRollsBackWhenRunUpdateFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	run, err := store.StartMCPRun("session-1", "rollback test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_mcp_run_event_count
BEFORE UPDATE OF event_count ON mcp_runs
WHEN OLD.id = '` + run.ID + `'
BEGIN
  SELECT RAISE(ABORT, 'forced run update failure');
END`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AddMCPRunEvent(run.ID, RunEventInput{Status: "success", Tool: "DockMini__task_manage"}); err == nil {
		t.Fatal("expected run update failure")
	}
	var eventCount, storedEventCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM mcp_run_events WHERE run_id = ?`, run.ID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT event_count FROM mcp_runs WHERE id = ?`, run.ID).Scan(&storedEventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 || storedEventCount != 0 {
		t.Fatalf("partial run event persisted: events=%d event_count=%d", eventCount, storedEventCount)
	}
}
