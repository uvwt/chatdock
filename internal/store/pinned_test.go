package store

import (
	"testing"
	"time"

	"chatdock/internal/model"
)

func TestListPinnedFeedReturnsOnlyPinnedItems(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	plainSession, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	appendUserMessageForTest(t, store, plainSession.ID, "普通会话")

	pinnedSession, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	appendUserMessageForTest(t, store, pinnedSession.ID, "置顶会话")
	if _, err := store.PinSession(pinnedSession.ID, true); err != nil {
		t.Fatal(err)
	}

	plainProject, err := store.CreateProject(model.CreateProjectRequest{Name: "普通项目"})
	if err != nil {
		t.Fatal(err)
	}
	pinnedProject, err := store.CreateProject(model.CreateProjectRequest{Name: "置顶项目"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PinProject(pinnedProject.ID, true); err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:        "普通任务",
		Prompt:       "hello",
		Enabled:      true,
		ScheduleType: "interval",
		IntervalMinutes: 60,
		ContextMode:  model.ScheduledTaskContextStateless,
	}); err != nil {
		t.Fatal(err)
	}
	pinnedTaskResp, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "置顶任务",
		Prompt:          "hello pinned",
		Enabled:         true,
		ScheduleType:    "interval",
		IntervalMinutes: 30,
		ContextMode:     model.ScheduledTaskContextStateless,
	})
	if err != nil {
		t.Fatal(err)
	}
	var pinnedTaskID string
	for _, task := range pinnedTaskResp.Tasks {
		if task.Title == "置顶任务" {
			pinnedTaskID = task.ID
			break
		}
	}
	if pinnedTaskID == "" {
		t.Fatal("pinned task not created")
	}
	if _, err := store.PinScheduledTask(pinnedTaskID, true); err != nil {
		t.Fatal(err)
	}

	// Ensure plain project is still unpinned.
	_ = plainProject

	feed, err := store.ListPinnedFeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Sessions) != 1 || feed.Sessions[0].ID != pinnedSession.ID {
		t.Fatalf("sessions = %#v, want only %s", feed.Sessions, pinnedSession.ID)
	}
	if len(feed.Projects) != 1 || feed.Projects[0].ID != pinnedProject.ID {
		t.Fatalf("projects = %#v, want only %s", feed.Projects, pinnedProject.ID)
	}
	if len(feed.Tasks) != 1 || feed.Tasks[0].ID != pinnedTaskID {
		t.Fatalf("tasks = %#v, want only %s", feed.Tasks, pinnedTaskID)
	}
	if !feed.Sessions[0].Pinned || !feed.Projects[0].Pinned || !feed.Tasks[0].Pinned {
		t.Fatalf("feed items should be pinned: %#v", feed)
	}
	if feed.Sessions[0].UpdatedAt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("unexpected session updated_at: %v", feed.Sessions[0].UpdatedAt)
	}
}
