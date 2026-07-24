package store

import (
	"chatdock/internal/schedule"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/model"
)

func normalizeStoredCronExpressions(expressions []string) []string {
	out := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		expression = strings.TrimSpace(expression)
		if expression != "" {
			out = append(out, expression)
		}
	}
	return out
}

func normalizeScheduledTaskForDB(task model.ScheduledTask) model.ScheduledTask {
	task.ID = strings.TrimSpace(task.ID)
	task.Title = strings.TrimSpace(task.Title)
	task.Prompt = strings.TrimSpace(task.Prompt)
	task.ScheduleType = strings.TrimSpace(task.ScheduleType)
	task.CronExpressions = normalizeStoredCronExpressions(task.CronExpressions)
	task.Timezone = strings.TrimSpace(task.Timezone)
	task.ContextMode = schedule.NormalizeContextMode(task.ContextMode)
	task.LastStatus = strings.TrimSpace(task.LastStatus)
	task.LastError = strings.TrimSpace(task.LastError)
	task.SessionID = strings.TrimSpace(task.SessionID)
	if task.ID == "" {
		task.ID = model.NewID()
	}
	if task.ScheduleType == "" {
		task.ScheduleType = scheduleTypeOnce
	}
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	return task
}

func normalizeScheduledRunRecordForDB(record model.ScheduledTaskRunRecord) model.ScheduledTaskRunRecord {
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		record.ID = model.NewID()
	}
	record.TaskID = strings.TrimSpace(record.TaskID)
	record.TaskTitle = strings.TrimSpace(record.TaskTitle)
	record.Prompt = strings.TrimSpace(record.Prompt)
	record.Output = strings.TrimSpace(record.Output)
	record.Status = strings.TrimSpace(record.Status)
	if record.Status == "" {
		record.Status = "success"
	}
	record.Error = strings.TrimSpace(record.Error)
	record.SessionID = strings.TrimSpace(record.SessionID)
	if record.StartedAt.IsZero() {
		record.StartedAt = time.Now()
	}
	if record.FinishedAt != nil && record.DurationMS <= 0 {
		record.DurationMS = record.FinishedAt.Sub(record.StartedAt).Milliseconds()
	}
	return record
}

func upsertScheduledTaskTx(tx sqlWriter, task model.ScheduledTask) error {
	cronExpressions, err := json.Marshal(task.CronExpressions)
	if err != nil {
		return fmt.Errorf("encode scheduled task cron expressions: %w", err)
	}
	_, err = tx.Exec(`INSERT INTO scheduled_tasks(id, title, task_prompt, pinned, enabled, running, schedule_type, run_at, cron_expressions, timezone, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET title = excluded.title, task_prompt = excluded.task_prompt, pinned = excluded.pinned, enabled = excluded.enabled, running = excluded.running, schedule_type = excluded.schedule_type, run_at = excluded.run_at, cron_expressions = excluded.cron_expressions, timezone = excluded.timezone, interval_minutes = excluded.interval_minutes, context_mode = excluded.context_mode, next_run_at = excluded.next_run_at, last_run_at = excluded.last_run_at, last_status = excluded.last_status, last_error = excluded.last_error, session_id = excluded.session_id, created_at = excluded.created_at, updated_at = excluded.updated_at`,
		task.ID, task.Title, task.Prompt, boolInt(task.Pinned), boolInt(task.Enabled), boolInt(task.Running), task.ScheduleType, formatOptionalTime(task.RunAt), string(cronExpressions), task.Timezone, task.IntervalMinutes, task.ContextMode, formatScheduleDBTime(task.NextRunAt), formatOptionalTime(task.LastRunAt), task.LastStatus, task.LastError, task.SessionID, formatScheduleDBTime(task.CreatedAt), formatScheduleDBTime(task.UpdatedAt))
	return err
}

func upsertScheduledTaskRunTx(tx interface {
	Exec(string, ...any) (sql.Result, error)
}, record model.ScheduledTaskRunRecord) error {
	_, err := tx.Exec(`INSERT INTO scheduled_task_runs(id, task_id, task_title, task_prompt, output, status, error, manual, session_id, started_at, finished_at, duration_ms) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET task_id = excluded.task_id, task_title = excluded.task_title, task_prompt = excluded.task_prompt, output = excluded.output, status = excluded.status, error = excluded.error, manual = excluded.manual, session_id = excluded.session_id, started_at = excluded.started_at, finished_at = excluded.finished_at, duration_ms = excluded.duration_ms`,
		record.ID, record.TaskID, record.TaskTitle, record.Prompt, record.Output, record.Status, record.Error, boolInt(record.Manual), record.SessionID, formatScheduleDBTime(record.StartedAt), formatOptionalTime(record.FinishedAt), record.DurationMS)
	return err
}

func loadScheduledTasksLocked(reader sqlQueryer) ([]model.ScheduledTask, error) {
	rows, err := reader.Query(`SELECT id, title, task_prompt, pinned, enabled, running, schedule_type, run_at, cron_expressions, timezone, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at FROM scheduled_tasks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledTasks(rows)
}

func scanScheduledTasks(rows *sql.Rows) ([]model.ScheduledTask, error) {
	out := []model.ScheduledTask{}
	for rows.Next() {
		task, err := scanScheduledTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	schedule.SortTasks(out)
	return out, nil
}

func scanScheduledTask(scanner interface{ Scan(...any) error }) (model.ScheduledTask, error) {
	var task model.ScheduledTask
	var pinned, enabled, running int
	var runAt, cronExpressions, nextRunAt, lastRunAt, createdAt, updatedAt string
	if err := scanner.Scan(&task.ID, &task.Title, &task.Prompt, &pinned, &enabled, &running, &task.ScheduleType, &runAt, &cronExpressions, &task.Timezone, &task.IntervalMinutes, &task.ContextMode, &nextRunAt, &lastRunAt, &task.LastStatus, &task.LastError, &task.SessionID, &createdAt, &updatedAt); err != nil {
		return model.ScheduledTask{}, err
	}
	if err := json.Unmarshal([]byte(cronExpressions), &task.CronExpressions); err != nil {
		return model.ScheduledTask{}, fmt.Errorf("decode scheduled task %s cron expressions: %w", task.ID, err)
	}
	task.Pinned = pinned != 0
	task.Enabled = enabled != 0
	task.Running = running != 0
	task.RunAt = parseOptionalDBTime(runAt)
	task.NextRunAt = parseDBTimeZero(nextRunAt)
	task.LastRunAt = parseOptionalDBTime(lastRunAt)
	task.CreatedAt = parseDBTimeZero(createdAt)
	task.UpdatedAt = parseDBTimeZero(updatedAt)
	task.ContextMode = schedule.NormalizeContextMode(task.ContextMode)
	return task, nil
}

func scheduledTaskRunColumns() string {
	return `id, task_id, task_title, task_prompt, output, status, error, manual, session_id, started_at, finished_at, duration_ms`
}

func scanScheduledTaskRun(scanner interface{ Scan(...any) error }) (model.ScheduledTaskRunRecord, error) {
	var record model.ScheduledTaskRunRecord
	var manual int
	var startedAt, finishedAt string
	if err := scanner.Scan(&record.ID, &record.TaskID, &record.TaskTitle, &record.Prompt, &record.Output, &record.Status, &record.Error, &manual, &record.SessionID, &startedAt, &finishedAt, &record.DurationMS); err != nil {
		return model.ScheduledTaskRunRecord{}, err
	}
	record.Manual = manual != 0
	record.StartedAt = parseDBTimeZero(startedAt)
	record.FinishedAt = parseOptionalDBTime(finishedAt)
	return record, nil
}

func scanScheduledTaskRunWithSessionTitle(scanner interface{ Scan(...any) error }) (model.ScheduledTaskRunRecord, error) {
	var record model.ScheduledTaskRunRecord
	var manual int
	var startedAt, finishedAt string
	if err := scanner.Scan(&record.ID, &record.TaskID, &record.TaskTitle, &record.Prompt, &record.Output, &record.Status, &record.Error, &manual, &record.SessionID, &startedAt, &finishedAt, &record.DurationMS, &record.SessionTitle); err != nil {
		return model.ScheduledTaskRunRecord{}, err
	}
	record.Manual = manual != 0
	record.StartedAt = parseDBTimeZero(startedAt)
	record.FinishedAt = parseOptionalDBTime(finishedAt)
	return record, nil
}

func formatScheduleDBTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return formatScheduleDBTime(*t)
}

func parseDBTimeZero(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t
	}
	return time.Time{}
}

func parseOptionalDBTime(value string) *time.Time {
	t := parseDBTimeZero(value)
	if t.IsZero() {
		return nil
	}
	return &t
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func deleteScheduledTasksExceptLocked(db sqlQueryWriter, keep map[string]bool) error {
	rows, err := db.Query(`SELECT id FROM scheduled_tasks`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if keep[id] {
			continue
		}
		if _, err := db.Exec(`DELETE FROM scheduled_tasks WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}
