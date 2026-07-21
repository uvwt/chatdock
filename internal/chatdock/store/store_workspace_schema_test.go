package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestSQLiteWorkspaceSchemaMigratesLegacyPromptTables(t *testing.T) {
	t.Setenv("CHATDOCK_TIMEZONE", "Asia/Shanghai")
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dataDir, "chatdock.sqlite")+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	legacyStatements := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO meta(key, value) VALUES('json_migrated', '1'), ('scheduled_tables_migrated', '1'), ('session_tables_migrated', '1')`,
		`CREATE TABLE prompts (name TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE prompt_kv (prompt TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(prompt, key), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE TABLE sessions (prompt TEXT NOT NULL, id TEXT NOT NULL, json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(prompt, id), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX idx_sessions_prompt_updated ON sessions(prompt, updated_at DESC)`,
		`CREATE TABLE scheduled_tasks (prompt TEXT NOT NULL, id TEXT NOT NULL, title TEXT NOT NULL, task_prompt TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, running INTEGER NOT NULL DEFAULT 0, schedule_type TEXT NOT NULL, run_at TEXT NOT NULL DEFAULT '', time_of_day TEXT NOT NULL DEFAULT '', interval_minutes INTEGER NOT NULL DEFAULT 0, context_mode TEXT NOT NULL DEFAULT 'stateless', next_run_at TEXT NOT NULL DEFAULT '', last_run_at TEXT NOT NULL DEFAULT '', last_status TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '', session_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(prompt, id), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX idx_scheduled_tasks_prompt_updated ON scheduled_tasks(prompt, updated_at DESC)`,
		`INSERT INTO prompts(name, created_at, updated_at) VALUES('default', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'), ('research', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`,
		`INSERT INTO prompt_kv(prompt, key, value, updated_at) VALUES('research', 'config', '{"model":"demo","system_prompt":"研究"}', '2026-01-02T00:00:00Z')`,
		`INSERT INTO sessions(prompt, id, json, created_at, updated_at) VALUES('research', 's1', '{"id":"s1","messages":[]}', '2026-01-02T00:00:00Z', '2026-01-02T00:00:00Z')`,
		`INSERT INTO scheduled_tasks(prompt, id, title, task_prompt, enabled, schedule_type, interval_minutes, next_run_at, created_at, updated_at) VALUES('research', 'task1', '日报', '总结', 1, 'interval', 60, '2026-01-02T01:00:00Z', '2026-01-02T00:00:00Z', '2026-01-02T00:00:00Z')`,
		`INSERT INTO scheduled_tasks(prompt, id, title, task_prompt, enabled, schedule_type, time_of_day, next_run_at, created_at, updated_at) VALUES('research', 'task2', '早晚简报', '总结', 1, 'daily', '09:30', '2026-01-02T01:30:00Z', '2026-01-02T00:00:00Z', '2026-01-02T00:00:00Z')`,
	}
	for _, stmt := range legacyStatements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("legacy stmt failed: %s: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, oldName := range []string{"prompts", "prompt_kv"} {
		exists, err := sqliteTableExists(store.db, oldName)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("legacy table should be removed after migration: %s", oldName)
		}
	}
	for _, table := range []string{"workspace_kv", "sessions", "scheduled_tasks"} {
		hasOld, err := sqliteColumnExists(store.db, table, "prompt")
		if err != nil {
			t.Fatal(err)
		}
		if hasOld {
			t.Fatalf("legacy prompt column remains on %s", table)
		}
		hasWorkspace, err := sqliteColumnExists(store.db, table, "workspace_id")
		if err != nil {
			t.Fatal(err)
		}
		if !hasWorkspace {
			t.Fatalf("workspace_id column missing on %s", table)
		}
	}
	hasTimeOfDay, err := sqliteColumnExists(store.db, "scheduled_tasks", "time_of_day")
	if err != nil {
		t.Fatal(err)
	}
	if hasTimeOfDay {
		t.Fatal("legacy time_of_day column remains after cron migration")
	}
	for _, column := range []string{"cron_expressions", "timezone"} {
		hasColumn, err := sqliteColumnExists(store.db, "scheduled_tasks", column)
		if err != nil {
			t.Fatal(err)
		}
		if !hasColumn {
			t.Fatalf("cron migration column missing: %s", column)
		}
	}

	var oldIndexCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name LIKE '%prompt%'`).Scan(&oldIndexCount); err != nil {
		t.Fatal(err)
	}
	if oldIndexCount != 0 {
		t.Fatalf("legacy prompt index names remain: %d", oldIndexCount)
	}
	var workspaceCount, kvCount, sessionCount, taskCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE name = 'research'`).Scan(&workspaceCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM workspace_kv WHERE workspace_id = 'research' AND key = 'config'`).Scan(&kvCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE workspace_id = 'research' AND id = 's1'`).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM scheduled_tasks WHERE workspace_id = 'research'`).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	var scheduleType, cronExpressions, timezone string
	if err := store.db.QueryRow(`SELECT schedule_type, cron_expressions, timezone FROM scheduled_tasks WHERE workspace_id = 'research' AND id = 'task2'`).Scan(&scheduleType, &cronExpressions, &timezone); err != nil {
		t.Fatal(err)
	}
	if scheduleType != "cron" || cronExpressions != `["30 9 * * *"]` || timezone != "Asia/Shanghai" {
		t.Fatalf("legacy daily row was not converted: type=%q cron=%q timezone=%q", scheduleType, cronExpressions, timezone)
	}
	if workspaceCount != 1 || kvCount != 1 || sessionCount != 1 || taskCount != 2 {
		t.Fatalf("migrated data mismatch: workspace=%d kv=%d sessions=%d tasks=%d", workspaceCount, kvCount, sessionCount, taskCount)
	}
}
