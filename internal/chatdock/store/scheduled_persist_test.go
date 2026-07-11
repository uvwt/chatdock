package store

import (
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestNextDailyRunUsesConfiguredScheduleTimezone(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	now := time.Date(2026, 7, 6, 2, 50, 0, 0, time.UTC) // 10:50 in Asia/Shanghai.
	got := nextDailyRun(now, "20:30")
	loc := scheduleLocation()
	if got.In(loc).Format("2006-01-02 15:04") != "2026-07-06 20:30" {
		t.Fatalf("next daily run = %s, want 2026-07-06 20:30 in %s", got.In(loc), loc)
	}
	if got.UTC().Format(time.RFC3339) != "2026-07-06T12:30:00Z" {
		t.Fatalf("next daily run UTC = %s, want 2026-07-06T12:30:00Z", got.UTC().Format(time.RFC3339))
	}
}

func TestRepairDailyNextRunFixesOldUTCStoredTime(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	task := model.ScheduledTask{
		ScheduleType: scheduleTypeDaily,
		TimeOfDay:    "20:30",
		NextRunAt:    time.Date(2026, 7, 6, 20, 30, 0, 0, time.UTC), // Old bug: 20:30 UTC, 04:30 local next day.
	}
	now := time.Date(2026, 7, 6, 2, 50, 0, 0, time.UTC)
	if !repairScheduledTaskNextRun(&task, now) {
		t.Fatal("expected repair for UTC-stored daily next_run_at")
	}
	loc := scheduleLocation()
	if task.NextRunAt.In(loc).Format("2006-01-02 15:04") != "2026-07-06 20:30" {
		t.Fatalf("repaired next_run_at = %s, want local 2026-07-06 20:30", task.NextRunAt.In(loc))
	}
}

func TestParseTaskTimeUsesScheduleTimezoneForLocalInput(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	got, err := parseTaskTime("2026-07-06T09:30")
	if err != nil {
		t.Fatal(err)
	}
	loc := scheduleLocation()
	if got.In(loc).Format("2006-01-02 15:04") != "2026-07-06 09:30" {
		t.Fatalf("parsed local run time = %s", got.In(loc))
	}
	if got.UTC().Format(time.RFC3339) != "2026-07-06T01:30:00Z" {
		t.Fatalf("parsed UTC run time = %s, want 2026-07-06T01:30:00Z", got.UTC().Format(time.RFC3339))
	}
}

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
		if _, err := store.CreateScheduledTask(defaultWorkspaceID, model.ScheduledTaskRequest{Title: title, Prompt: title, Enabled: true, ScheduleType: scheduleTypeInterval, IntervalMinutes: 30}); err != nil {
			t.Fatal(err)
		}
	}
	store.mu.Lock()
	tasks, err := store.loadScheduledTasksForWorkspaceLocked(defaultWorkspaceID)
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
	sortScheduledTasks(tasks)
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
	err = store.saveScheduledTasksForWorkspaceLocked(defaultWorkspaceID, tasks)
	store.mu.Unlock()
	if err == nil {
		t.Fatal("expected batch save failure")
	}
	rows, err := store.db.Query(`SELECT id, title FROM scheduled_tasks WHERE workspace_id = ?`, defaultWorkspaceID)
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

func TestFinishScheduledTaskRunRollsBackSessionAndTaskTogether(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	created, err := store.CreateScheduledTask(defaultWorkspaceID, model.ScheduledTaskRequest{
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
	run, err := store.PrepareScheduledTaskRunInWorkspace(defaultWorkspaceID, created.Tasks[0].ID, true, startedAt)
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

	_, err = store.FinishScheduledTaskRun(defaultWorkspaceID, run.Task.ID, run.RunID, run.SessionID, "不应部分保存", startedAt, true, nil, false)
	if err == nil {
		t.Fatal("expected scheduled completion failure")
	}
	session, ok, err := store.GetSession(defaultWorkspaceID, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(session.Messages) != 1 || session.Messages[0].Role != "user" {
		t.Fatalf("assistant message survived completion rollback: %#v", session)
	}
	var running int
	if err := store.db.QueryRow(`SELECT running FROM scheduled_tasks WHERE workspace_id = ? AND id = ?`, defaultWorkspaceID, run.Task.ID).Scan(&running); err != nil {
		t.Fatal(err)
	}
	if running != 1 {
		t.Fatalf("task running state changed despite rollback: %d", running)
	}
	var runCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM scheduled_task_runs WHERE workspace_id = ? AND id = ?`, defaultWorkspaceID, run.RunID).Scan(&runCount); err != nil {
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
	created, err := store.CreateScheduledTask(defaultWorkspaceID, model.ScheduledTaskRequest{
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
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE workspace_id = ?`, defaultWorkspaceID).Scan(&beforeSessions); err != nil {
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

	if _, err := store.PrepareScheduledTaskRunInWorkspace(defaultWorkspaceID, task.ID, true, time.Now()); err == nil {
		t.Fatal("expected scheduled start failure")
	}
	var afterSessions int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE workspace_id = ?`, defaultWorkspaceID).Scan(&afterSessions); err != nil {
		t.Fatal(err)
	}
	if afterSessions != beforeSessions {
		t.Fatalf("scheduled session survived start rollback: before=%d after=%d", beforeSessions, afterSessions)
	}
	var running int
	var sessionID string
	if err := store.db.QueryRow(`SELECT running, session_id FROM scheduled_tasks WHERE workspace_id = ? AND id = ?`, defaultWorkspaceID, task.ID).Scan(&running, &sessionID); err != nil {
		t.Fatal(err)
	}
	if running != 0 || sessionID != "" {
		t.Fatalf("task state survived start rollback: running=%d session_id=%q", running, sessionID)
	}
}
