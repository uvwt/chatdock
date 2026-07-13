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

	created, err := store.CreateScheduledTask(defaultWorkspaceID, model.ScheduledTaskRequest{
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
	run, err := store.PrepareScheduledTaskRunInWorkspace(defaultWorkspaceID, created.Tasks[0].ID, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID == "" {
		t.Fatal("scheduled run should create a session")
	}

	summaries, err := store.ListSessions(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Fatalf("running scheduled session should be hidden: %#v", summaries)
	}
	searchResults, err := store.SearchSessions(defaultWorkspaceID, "隐藏会话报告", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(searchResults) != 0 {
		t.Fatalf("running scheduled session should be hidden from search: %#v", searchResults)
	}
}
