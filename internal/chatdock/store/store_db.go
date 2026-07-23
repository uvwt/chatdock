package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// SQLite 初始化只创建当前版本的完整 schema；旧数据库必须先通过外部一次性迁移工具转换。

type sqlWriter interface {
	Exec(string, ...any) (sql.Result, error)
}

type sqlQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

type sqlQueryWriter interface {
	sqlWriter
	sqlQueryer
}

func (s *Store) initSQLite() error {
	bootstrap := []string{
		`PRAGMA journal_mode = DELETE`,
		`PRAGMA synchronous = FULL`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	}
	for _, stmt := range bootstrap {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := rejectLegacyWorkspaceSchema(s.db); err != nil {
		return err
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS global_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS projects (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, prompt TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, project_id TEXT NULL, title TEXT NOT NULL DEFAULT '', pinned INTEGER NOT NULL DEFAULT 0, provider_id TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE SET NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_project_updated ON sessions(project_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS session_messages (session_id TEXT NOT NULL, message_index INTEGER NOT NULL, id TEXT NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL, reasoning TEXT NOT NULL DEFAULT '', error_json TEXT NOT NULL DEFAULT '', attachments_json TEXT NOT NULL DEFAULT '[]', created_at TEXT NOT NULL, PRIMARY KEY(session_id, message_index), FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_messages_id ON session_messages(session_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_session_messages_session ON session_messages(session_id, message_index)`,
		`CREATE TABLE IF NOT EXISTS session_message_parts (session_id TEXT NOT NULL, message_index INTEGER NOT NULL, part_index INTEGER NOT NULL, kind TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', event_id TEXT NOT NULL DEFAULT '', PRIMARY KEY(session_id, message_index, part_index), FOREIGN KEY(session_id, message_index) REFERENCES session_messages(session_id, message_index) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS session_message_events (session_id TEXT NOT NULL, message_index INTEGER NOT NULL, event_index INTEGER NOT NULL, id TEXT NOT NULL, kind TEXT NOT NULL, phase TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', text TEXT NOT NULL DEFAULT '', meta TEXT NOT NULL DEFAULT '', PRIMARY KEY(session_id, message_index, event_index), FOREIGN KEY(session_id, message_index) REFERENCES session_messages(session_id, message_index) ON DELETE CASCADE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_message_events_id ON session_message_events(session_id, id)`,
		`CREATE TABLE IF NOT EXISTS session_message_event_details (session_id TEXT NOT NULL, event_id TEXT NOT NULL, details_json TEXT NOT NULL, details_bytes INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, PRIMARY KEY(session_id, event_id), FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_session_event_details_bytes ON session_message_event_details(details_bytes DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, title TEXT NOT NULL, status TEXT NOT NULL, summary TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, event_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_updated ON mcp_runs(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_session_updated ON mcp_runs(session_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_run_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, server TEXT NOT NULL, tool TEXT NOT NULL, action TEXT NOT NULL, summary TEXT NOT NULL, arguments_json TEXT NOT NULL, result_json TEXT NOT NULL, error TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(run_id) REFERENCES mcp_runs(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS mcp_confirmations (id TEXT PRIMARY KEY, session_id TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL, arguments_json TEXT NOT NULL DEFAULT '{}', status TEXT NOT NULL, requested_at TEXT NOT NULL, resolved_at TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_confirmations_status_requested ON mcp_confirmations(status, requested_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_run_seq ON mcp_run_events(run_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_tool_created ON mcp_run_events(tool, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS chat_jobs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, request_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, answer TEXT NOT NULL, reasoning TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_jobs_session_updated ON chat_jobs(session_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_jobs_status_updated ON chat_jobs(status, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS chat_job_events (job_id TEXT NOT NULL, seq INTEGER NOT NULL, event TEXT NOT NULL, data_json TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(job_id, seq), FOREIGN KEY(job_id) REFERENCES chat_jobs(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_job_events_job_event_seq ON chat_job_events(job_id, event, seq)`,
		`CREATE TABLE IF NOT EXISTS scheduled_tasks ` + scheduledTasksTableColumnsSQL,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_due ON scheduled_tasks(enabled, running, next_run_at)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_updated ON scheduled_tasks(updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS scheduled_task_runs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, task_title TEXT NOT NULL, task_prompt TEXT NOT NULL, output TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', manual INTEGER NOT NULL DEFAULT 0, session_id TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL, finished_at TEXT NOT NULL DEFAULT '', duration_ms INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_task_started ON scheduled_task_runs(task_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_started ON scheduled_task_runs(started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS attachment_blobs (sha256 TEXT PRIMARY KEY, storage_path TEXT NOT NULL, size INTEGER NOT NULL, mime_type TEXT NOT NULL, ref_count INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_attachment_blobs_created ON attachment_blobs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS attachments (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, message_id TEXT NOT NULL, filename TEXT NOT NULL, mime_type TEXT NOT NULL, size INTEGER NOT NULL, storage_path TEXT NOT NULL, sha256 TEXT NOT NULL, text_content TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_session ON attachments(session_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS tool_embeddings (full_name TEXT NOT NULL, source_hash TEXT NOT NULL, embedding_model TEXT NOT NULL, embedding_json TEXT NOT NULL, embedding_blob BLOB NOT NULL DEFAULT x'', indexed_at TEXT NOT NULL, PRIMARY KEY(full_name, embedding_model))`,
		`CREATE INDEX IF NOT EXISTS idx_tool_embeddings_model ON tool_embeddings(embedding_model)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func rejectLegacyWorkspaceSchema(db *sql.DB) error {
	for _, table := range []string{"workspaces", "workspace_kv", "prompts", "prompt_kv"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("legacy workspace schema detected: table %s exists; run the external one-time migration before starting ChatDock", table)
		}
	}
	for _, table := range []string{"sessions", "session_messages", "session_message_parts", "session_message_events", "session_message_event_details", "mcp_runs", "mcp_confirmations", "chat_jobs", "scheduled_tasks", "scheduled_task_runs", "attachments", "tool_embeddings"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil || !exists {
			if err != nil {
				return err
			}
			continue
		}
		hasWorkspace, err := sqliteColumnExists(db, table, "workspace_id")
		if err != nil {
			return err
		}
		if hasWorkspace {
			return fmt.Errorf("legacy workspace schema detected: column %s.workspace_id exists; run the external one-time migration before starting ChatDock", table)
		}
		hasPrompt, err := sqliteColumnExists(db, table, "prompt")
		if err != nil {
			return err
		}
		if hasPrompt {
			return fmt.Errorf("legacy prompt schema detected: column %s.prompt exists; run the external one-time migration before starting ChatDock", table)
		}
	}
	return nil
}

func sqliteTableExists(q interface{ QueryRow(string, ...any) *sql.Row }, name string) (bool, error) {
	var got string
	err := q.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func sqliteColumnExists(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, table string, column string) (bool, error) {
	rows, err := q.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) metaValue(key string) (string, error) {
	return metaValueWith(s.db, key)
}

func metaValueWith(reader sqlQueryer, key string) (string, error) {
	var value string
	err := reader.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) setMetaValue(key string, value string) error {
	return setMetaValueWith(s.db, key, value)
}

func setMetaValueWith(writer sqlWriter, key string, value string) error {
	_, err := writer.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
