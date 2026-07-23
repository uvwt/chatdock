package store

import (
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestNewStoreRecoversInterruptedWork(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	job, err := createChatJobForTest(t, store, session.ID, "req-restart")
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateScheduledTask(scheduledTaskRequestForTest("restart-task"))
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 {
		t.Fatalf("unexpected scheduled tasks: %#v", created.Tasks)
	}
	taskID := created.Tasks[0].ID
	now := formatDBTime(time.Now())
	if _, err := store.db.Exec(`UPDATE scheduled_tasks SET running = 1, last_status = '', last_error = '' WHERE id = ?`, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO mcp_runs(id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at) VALUES(?, ?, ?, 'running', '', '', ?, '', 0, 0, ?)`, "run-restart", session.ID, "restart run", now, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	var jobStatus, jobError, jobFinished string
	if err := reopened.db.QueryRow(`SELECT status, error, finished_at FROM chat_jobs WHERE id = ?`, job.ID).Scan(&jobStatus, &jobError, &jobFinished); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "interrupted" || jobError != interruptedByRestartMessage || jobFinished == "" {
		t.Fatalf("chat job not recovered: status=%q error=%q finished=%q", jobStatus, jobError, jobFinished)
	}
	var running int
	var taskStatus, taskError string
	if err := reopened.db.QueryRow(`SELECT running, last_status, last_error FROM scheduled_tasks WHERE id = ?`, taskID).Scan(&running, &taskStatus, &taskError); err != nil {
		t.Fatal(err)
	}
	if running != 0 || taskStatus != "error" || taskError != interruptedByRestartMessage {
		t.Fatalf("scheduled task not recovered: running=%d status=%q error=%q", running, taskStatus, taskError)
	}
	var runStatus, runError, runFinished string
	if err := reopened.db.QueryRow(`SELECT status, error, finished_at FROM mcp_runs WHERE id = 'run-restart'`).Scan(&runStatus, &runError, &runFinished); err != nil {
		t.Fatal(err)
	}
	if runStatus != "error" || runError != interruptedByRestartMessage || runFinished == "" {
		t.Fatalf("MCP run not recovered: status=%q error=%q finished=%q", runStatus, runError, runFinished)
	}
}

func scheduledTaskRequestForTest(title string) model.ScheduledTaskRequest {
	return model.ScheduledTaskRequest{
		Title:           title,
		Prompt:          title,
		Enabled:         true,
		ScheduleType:    scheduleTypeInterval,
		IntervalMinutes: 30,
	}
}

func TestRecoverInterruptedWorkIsIdempotentAndPreservesTerminalRows(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := formatDBTime(time.Now())
	if _, err := store.db.Exec(`INSERT INTO chat_jobs(id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at) VALUES('completed-job', '', '', 'completed', 'done', '', '', ?, ?, ?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO mcp_confirmations(id, session_id, tool, arguments_json, status, requested_at, resolved_at, message) VALUES('approved-confirmation', '', 'demo', '{}', 'approved', ?, ?, 'approved')`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := store.recoverInterruptedWork(); err != nil {
		t.Fatal(err)
	}
	if err := store.recoverInterruptedWork(); err != nil {
		t.Fatal(err)
	}

	var jobStatus, jobAnswer string
	if err := store.db.QueryRow(`SELECT status, answer FROM chat_jobs WHERE id = 'completed-job'`).Scan(&jobStatus, &jobAnswer); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "completed" || jobAnswer != "done" {
		t.Fatalf("terminal chat job changed: status=%q answer=%q", jobStatus, jobAnswer)
	}
	var confirmationStatus, confirmationMessage string
	if err := store.db.QueryRow(`SELECT status, message FROM mcp_confirmations WHERE id = 'approved-confirmation'`).Scan(&confirmationStatus, &confirmationMessage); err != nil {
		t.Fatal(err)
	}
	if confirmationStatus != "approved" || confirmationMessage != "approved" {
		t.Fatalf("terminal confirmation changed: status=%q message=%q", confirmationStatus, confirmationMessage)
	}
}
