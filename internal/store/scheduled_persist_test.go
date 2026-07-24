package store

import (
	"testing"
	"time"

	"chatdock/internal/model"
	"chatdock/internal/schedule"
)

func TestSaveScheduledTasksRollsBackPartialBatch(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	for _, title := range []string{"任务一", "任务二"} {
		if _, err := store.CreateScheduledTask(model.ScheduledTaskRequest{Title: title, Prompt: title, Enabled: true, ScheduleType: scheduleTypeInterval, IntervalMinutes: 30}); err != nil {
			t.Fatal(err)
		}
	}
	store.mu.Lock()
	tasks, err := store.loadScheduledTasksLocked()
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("unexpected tasks: %#v", tasks)
	}
	originalTitles := map[string]string{}
	for i := range tasks {
		originalTitles[tasks[i].ID] = tasks[i].Title
		tasks[i].Title += "-已修改"
	}
	schedule.SortTasks(tasks)
	failingID := tasks[len(tasks)-1].ID
	if _, err := store.db.Exec(`CREATE TRIGGER fail_scheduled_task_batch
BEFORE UPDATE ON scheduled_tasks
WHEN OLD.id = '` + failingID + `'
BEGIN
  SELECT RAISE(ABORT, 'forced scheduled task failure');
END`); err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	err = store.saveScheduledTasksLocked(tasks)
	store.mu.Unlock()
	if err == nil {
		t.Fatal("expected batch save failure")
	}
	rows, err := store.db.Query(`SELECT id, title FROM scheduled_tasks`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			t.Fatal(err)
		}
		if title != originalTitles[id] {
			t.Fatalf("partial scheduled task update persisted: id=%s title=%q want=%q", id, title, originalTitles[id])
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestPinScheduledTaskPersistsAndSurvivesEditing(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title: "置顶任务", Prompt: "执行", Enabled: true,
		ScheduleType: scheduleTypeInterval, IntervalMinutes: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 {
		t.Fatalf("created tasks = %#v", created.Tasks)
	}
	id := created.Tasks[0].ID
	pinned, err := store.PinScheduledTask(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pinned.Tasks) != 1 || !pinned.Tasks[0].Pinned {
		t.Fatalf("pinned tasks = %#v", pinned.Tasks)
	}

	updated, err := store.UpdateScheduledTask(id, model.ScheduledTaskRequest{
		Title: "置顶任务已编辑", Prompt: "继续执行", Enabled: true,
		ScheduleType: scheduleTypeInterval, IntervalMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Tasks) != 1 || !updated.Tasks[0].Pinned {
		t.Fatalf("editing lost pinned state: %#v", updated.Tasks)
	}

	reloaded, err := store.ListScheduledTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Tasks) != 1 || !reloaded.Tasks[0].Pinned {
		t.Fatalf("reloaded tasks = %#v", reloaded.Tasks)
	}
}

func TestFinishScheduledTaskRunRollsBackSessionAndTaskTogether(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "原子完成任务",
		Prompt:          "生成结果",
		Enabled:         true,
		ScheduleType:    scheduleTypeInterval,
		IntervalMinutes: 30,
		ContextMode:     model.ScheduledTaskContextSession,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 {
		t.Fatalf("unexpected tasks: %#v", created.Tasks)
	}
	startedAt := time.Now()
	run, err := store.PrepareScheduledTaskRun(created.Tasks[0].ID, true, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID == "" {
		t.Fatal("session context did not create a session")
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_scheduled_completion_update
BEFORE UPDATE ON scheduled_tasks
WHEN OLD.id = '` + run.Task.ID + `'
BEGIN
  SELECT RAISE(ABORT, 'forced scheduled completion failure');
END`); err != nil {
		t.Fatal(err)
	}

	_, err = store.FinishScheduledTaskRun(run.Task.ID, run.RunID, run.SessionID, "不应部分保存", startedAt, true, nil, false)
	if err == nil {
		t.Fatal("expected scheduled completion failure")
	}
	session, ok, err := store.GetSession(run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(session.Messages) != 1 || session.Messages[0].Role != "user" {
		t.Fatalf("assistant message survived completion rollback: %#v", session)
	}
	var running int
	if err := store.db.QueryRow(`SELECT running FROM scheduled_tasks WHERE id = ?`, run.Task.ID).Scan(&running); err != nil {
		t.Fatal(err)
	}
	if running != 1 {
		t.Fatalf("task running state changed despite rollback: %d", running)
	}
	var runCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM scheduled_task_runs WHERE id = ?`, run.RunID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 0 {
		t.Fatalf("run record survived completion rollback: %d", runCount)
	}
}

func TestPrepareScheduledTaskRunRollsBackSessionAndTaskTogether(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	created, err := store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "原子启动任务",
		Prompt:          "准备输入",
		Enabled:         true,
		ScheduleType:    scheduleTypeInterval,
		IntervalMinutes: 30,
		ContextMode:     model.ScheduledTaskContextSession,
	})
	if err != nil {
		t.Fatal(err)
	}
	task := created.Tasks[0]
	var beforeSessions int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&beforeSessions); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_scheduled_start_update
BEFORE UPDATE ON scheduled_tasks
WHEN OLD.id = '` + task.ID + `'
BEGIN
  SELECT RAISE(ABORT, 'forced scheduled start failure');
END`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.PrepareScheduledTaskRun(task.ID, true, time.Now()); err == nil {
		t.Fatal("expected scheduled start failure")
	}
	var afterSessions int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&afterSessions); err != nil {
		t.Fatal(err)
	}
	if afterSessions != beforeSessions {
		t.Fatalf("scheduled session survived start rollback: before=%d after=%d", beforeSessions, afterSessions)
	}
	var running int
	var sessionID string
	if err := store.db.QueryRow(`SELECT running, session_id FROM scheduled_tasks WHERE id = ?`, task.ID).Scan(&running, &sessionID); err != nil {
		t.Fatal(err)
	}
	if running != 0 || sessionID != "" {
		t.Fatalf("task state survived start rollback: running=%d session_id=%q", running, sessionID)
	}
}
