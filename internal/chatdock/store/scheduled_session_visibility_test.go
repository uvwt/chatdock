package store

import (
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestScheduledSessionIsHiddenWhileRunning(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "隐藏会话任务",
		Prompt:          "生成一份隐藏会话报告",
		Enabled:         true,
		ScheduleType:    scheduleTypeInterval,
		IntervalMinutes: 30,
		ContextMode:     model.ScheduledTaskContextStateless,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareScheduledTaskRun(created.Tasks[0].ID, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID == "" {
		t.Fatal("scheduled run should create a session")
	}

	summaries, err := store.ListSessions(SessionProjectFilter{Mode: SessionProjectFilterAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Fatalf("running scheduled session should be hidden: %#v", summaries)
	}
	searchResults, err := store.SearchSessions("隐藏会话报告", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(searchResults) != 0 {
		t.Fatalf("running scheduled session should be hidden from search: %#v", searchResults)
	}
}

func TestScheduledTaskRunListIncludesGeneratedSessionTitle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "每日早安",
		Prompt:          "生成今日晨间摘要",
		Enabled:         true,
		ScheduleType:    scheduleTypeInterval,
		IntervalMinutes: 30,
		ContextMode:     model.ScheduledTaskContextStateless,
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	run, err := store.PrepareScheduledTaskRun(created.Tasks[0].ID, true, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishScheduledTaskRun(run.Task.ID, run.RunID, run.SessionID, "今天适合专注开发。", startedAt, true, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RenameSession(run.SessionID, "今日晨间摘要"); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListScheduledTaskRuns(run.Task.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 || runs.Runs[0].SessionTitle != "今日晨间摘要" {
		t.Fatalf("scheduled run should expose generated session title: %#v", runs.Runs)
	}
}

func TestScheduledTaskRunListMasksMissingSession(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "遗留运行记录",
		Prompt:          "生成报告",
		Enabled:         true,
		ScheduleType:    scheduleTypeInterval,
		IntervalMinutes: 30,
		ContextMode:     model.ScheduledTaskContextStateless,
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	run, err := store.PrepareScheduledTaskRun(created.Tasks[0].ID, true, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishScheduledTaskRun(run.Task.ID, run.RunID, run.SessionID, "报告完成", startedAt, true, nil, false); err != nil {
		t.Fatal(err)
	}
	// 模拟历史数据：运行记录仍保留 session_id，但目标会话已不存在。
	if _, err := store.db.Exec(`DELETE FROM sessions WHERE id = ?`, run.SessionID); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListScheduledTaskRuns(run.Task.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 {
		t.Fatalf("run history should be preserved: %#v", runs.Runs)
	}
	if runs.Runs[0].SessionID != "" || runs.Runs[0].SessionTitle != "" {
		t.Fatalf("missing session must not be exposed as an actionable link: %#v", runs.Runs[0])
	}
}

func TestDeleteSessionClearsScheduledTaskReferences(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "连续上下文任务",
		Prompt:          "继续生成报告",
		Enabled:         true,
		ScheduleType:    scheduleTypeInterval,
		IntervalMinutes: 30,
		ContextMode:     model.ScheduledTaskContextSession,
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	run, err := store.PrepareScheduledTaskRun(created.Tasks[0].ID, true, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishScheduledTaskRun(run.Task.ID, run.RunID, run.SessionID, "报告完成", startedAt, true, nil, false); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.DeleteSession(run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("scheduled session should be deleted")
	}

	var taskSessionID, runSessionID string
	if err := store.db.QueryRow(`SELECT session_id FROM scheduled_tasks WHERE id = ?`, run.Task.ID).Scan(&taskSessionID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT session_id FROM scheduled_task_runs WHERE id = ?`, run.RunID).Scan(&runSessionID); err != nil {
		t.Fatal(err)
	}
	if taskSessionID != "" || runSessionID != "" {
		t.Fatalf("deleted session references survived: task=%q run=%q", taskSessionID, runSessionID)
	}
	nextRun, err := store.PrepareScheduledTaskRun(run.Task.ID, true, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if nextRun.SessionID == "" || nextRun.SessionID == run.SessionID {
		t.Fatalf("continuous task should create a replacement session: old=%q new=%q", run.SessionID, nextRun.SessionID)
	}
}
