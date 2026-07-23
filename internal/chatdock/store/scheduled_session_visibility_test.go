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
