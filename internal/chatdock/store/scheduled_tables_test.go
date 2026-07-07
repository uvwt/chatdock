package store

import (
	"encoding/json"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestScheduledJSONMigratesToTables(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.mu.Lock()
	if _, err := st.db.Exec(`DELETE FROM meta WHERE key = ?`, scheduledTablesMigratedKey); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	next := time.Date(2026, 7, 6, 12, 30, 0, 0, time.FixedZone("CST", 8*3600))
	legacyTasks := []model.ScheduledTask{{ID: "task-legacy", Title: "旧任务", Prompt: "执行旧任务", Enabled: true, ScheduleType: "daily", TimeOfDay: "12:30", ContextMode: model.ScheduledTaskContextStateless, NextRunAt: next, CreatedAt: next.Add(-time.Hour), UpdatedAt: next.Add(-time.Minute)}}
	rawTasks, _ := json.Marshal(legacyTasks)
	if err := st.setWorkspaceRawLocked(defaultWorkspaceID, "scheduled_tasks", string(rawTasks)); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	finished := next.Add(2 * time.Minute)
	legacyRuns := []model.ScheduledTaskRunRecord{{ID: "run-legacy", TaskID: "task-legacy", TaskTitle: "旧任务", Prompt: "执行旧任务", Output: "完成", Status: "success", StartedAt: next, FinishedAt: &finished, DurationMS: 120000}}
	rawRuns, _ := json.Marshal(legacyRuns)
	if err := st.setWorkspaceRawLocked(defaultWorkspaceID, scheduledTaskRunsKey, string(rawRuns)); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	if err := st.migrateScheduledJSONToTables(); err != nil {
		t.Fatal(err)
	}
	tasks, err := st.ListScheduledTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].ID != "task-legacy" || tasks.Tasks[0].Title != "旧任务" {
		t.Fatalf("unexpected migrated tasks: %#v", tasks.Tasks)
	}
	runs, err := st.ListScheduledTaskRuns("task-legacy", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 || runs.Runs[0].ID != "run-legacy" || runs.Runs[0].Output != "完成" {
		t.Fatalf("unexpected migrated runs: %#v", runs.Runs)
	}
	var taskRows, runRows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM scheduled_tasks`).Scan(&taskRows); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM scheduled_task_runs`).Scan(&runRows); err != nil {
		t.Fatal(err)
	}
	if taskRows != 1 || runRows != 1 {
		t.Fatalf("table row counts = tasks %d runs %d", taskRows, runRows)
	}
}

func TestScheduledTaskCRUDUsesTables(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	created, err := st.CreateScheduledTask(model.ScheduledTaskRequest{Title: "表任务", Prompt: "执行", Enabled: true, ScheduleType: "interval", IntervalMinutes: 15})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 {
		t.Fatalf("unexpected created tasks: %#v", created.Tasks)
	}
	id := created.Tasks[0].ID
	var rows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM scheduled_tasks WHERE id = ?`, id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("scheduled_tasks rows = %d, want 1", rows)
	}
	listed, err := st.ListScheduledTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tasks) != 1 || listed.Tasks[0].ID != id {
		t.Fatalf("unexpected listed tasks: %#v", listed.Tasks)
	}
}
