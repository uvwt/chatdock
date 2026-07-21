package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const scheduledTasksTableColumnsSQL = `(
	workspace_id TEXT NOT NULL,
	id TEXT NOT NULL,
	title TEXT NOT NULL,
	task_prompt TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 0,
	running INTEGER NOT NULL DEFAULT 0,
	schedule_type TEXT NOT NULL,
	run_at TEXT NOT NULL DEFAULT '',
	cron_expressions TEXT NOT NULL DEFAULT '[]',
	timezone TEXT NOT NULL DEFAULT '',
	interval_minutes INTEGER NOT NULL DEFAULT 0,
	context_mode TEXT NOT NULL DEFAULT 'stateless',
	next_run_at TEXT NOT NULL DEFAULT '',
	last_run_at TEXT NOT NULL DEFAULT '',
	last_status TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY(workspace_id, id),
	FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE
)`

func migrateScheduledTasksCronSchema(tx *sql.Tx) error {
	hasLegacyDailyColumn, err := sqliteColumnExists(tx, "scheduled_tasks", "time_of_day")
	if err != nil {
		return err
	}
	if !hasLegacyDailyColumn {
		return nil
	}

	// 旧 daily 数据只在迁移阶段读取；迁移完成后业务模型和 SQL 不再识别 daily/time_of_day。
	rows, err := tx.Query(`SELECT workspace_id, id, time_of_day FROM scheduled_tasks WHERE schedule_type = 'daily'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var workspaceID, taskID, timeOfDay string
		if err := rows.Scan(&workspaceID, &taskID, &timeOfDay); err != nil {
			rows.Close()
			return err
		}
		if _, err := time.Parse("15:04", strings.TrimSpace(timeOfDay)); err != nil {
			rows.Close()
			return fmt.Errorf("cannot migrate daily task %s/%s with time_of_day %q: %w", workspaceID, taskID, timeOfDay, err)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := tx.Exec(`CREATE TABLE scheduled_tasks_cron_migration ` + scheduledTasksTableColumnsSQL); err != nil {
		return err
	}
	timezone := configuredScheduleTimezone()
	_, err = tx.Exec(`INSERT INTO scheduled_tasks_cron_migration(
		workspace_id, id, title, task_prompt, enabled, running, schedule_type, run_at, cron_expressions, timezone, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at
	)
	SELECT
		workspace_id, id, title, task_prompt, enabled, running,
		CASE WHEN schedule_type = 'daily' THEN 'cron' ELSE schedule_type END,
		run_at,
		CASE WHEN schedule_type = 'daily' THEN
			'["' || CAST(substr(trim(time_of_day), 4, 2) AS INTEGER) || ' ' || CAST(substr(trim(time_of_day), 1, 2) AS INTEGER) || ' * * *"]'
		ELSE '[]' END,
		CASE WHEN schedule_type = 'daily' THEN ? ELSE '' END,
		interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at
	FROM scheduled_tasks`, timezone)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE scheduled_tasks`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE scheduled_tasks_cron_migration RENAME TO scheduled_tasks`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX idx_scheduled_tasks_due ON scheduled_tasks(enabled, running, next_run_at)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX idx_scheduled_tasks_workspace_updated ON scheduled_tasks(workspace_id, updated_at DESC)`); err != nil {
		return err
	}
	return nil
}
