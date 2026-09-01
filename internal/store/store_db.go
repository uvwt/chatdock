package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// SQLite 初始化创建当前 schema，并在启动时对旧数据库做无损增量升级。

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
		`CREATE TABLE IF NOT EXISTS projects (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, prompt TEXT NOT NULL DEFAULT '', pinned INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, project_id TEXT NULL, title TEXT NOT NULL DEFAULT '', pinned INTEGER NOT NULL DEFAULT 0, provider_id TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', system_prompt_snapshot TEXT NOT NULL DEFAULT '', project_prompt_snapshot TEXT NOT NULL DEFAULT '', system_prompt_frozen INTEGER NOT NULL DEFAULT 0, project_prompt_frozen INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE SET NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_project_updated ON sessions(project_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS session_messages (session_id TEXT NOT NULL, message_index INTEGER NOT NULL, id TEXT NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL, reasoning TEXT NOT NULL DEFAULT '', error_json TEXT NOT NULL DEFAULT '', usage_json TEXT NOT NULL DEFAULT '', attachments_json TEXT NOT NULL DEFAULT '[]', created_at TEXT NOT NULL, PRIMARY KEY(session_id, message_index), FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_messages_id ON session_messages(session_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_session_messages_session ON session_messages(session_id, message_index)`,
		`CREATE TABLE IF NOT EXISTS session_message_parts (session_id TEXT NOT NULL, message_index INTEGER NOT NULL, part_index INTEGER NOT NULL, kind TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', event_id TEXT NOT NULL DEFAULT '', PRIMARY KEY(session_id, message_index, part_index), FOREIGN KEY(session_id, message_index) REFERENCES session_messages(session_id, message_index) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS session_message_events (session_id TEXT NOT NULL, message_index INTEGER NOT NULL, event_index INTEGER NOT NULL, id TEXT NOT NULL, kind TEXT NOT NULL, phase TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', text TEXT NOT NULL DEFAULT '', meta TEXT NOT NULL DEFAULT '', PRIMARY KEY(session_id, message_index, event_index), FOREIGN KEY(session_id, message_index) REFERENCES session_messages(session_id, message_index) ON DELETE CASCADE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_message_events_id ON session_message_events(session_id, id)`,
		`CREATE TABLE IF NOT EXISTS session_tool_working_set (session_id TEXT NOT NULL, tool_name TEXT NOT NULL, resource_id TEXT NOT NULL, last_discovered_turn INTEGER NOT NULL DEFAULT 0, last_called_turn INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(session_id, tool_name), FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_session_tool_working_set_session ON session_tool_working_set(session_id, last_called_turn DESC, last_discovered_turn DESC)`,
		`CREATE TABLE IF NOT EXISTS session_context_checkpoints (session_id TEXT NOT NULL, provider_id TEXT NOT NULL, model TEXT NOT NULL, summary TEXT NOT NULL DEFAULT '', cutoff_message_id TEXT NOT NULL DEFAULT '', cutoff_message_index INTEGER NOT NULL DEFAULT -1, updated_at TEXT NOT NULL, PRIMARY KEY(session_id, provider_id, model), FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_session_context_checkpoints_session ON session_context_checkpoints(session_id, updated_at DESC)`,
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
	if err := ensurePinnedEntityColumns(s.db); err != nil {
		return err
	}
	if err := ensureContextSchemaColumns(s.db); err != nil {
		return err
	}
	if err := backfillMCPAppEventMeta(s.db); err != nil {
		return err
	}
	return nil
}

func ensureContextSchemaColumns(db *sql.DB) error {
	// 这些字段是无损增量迁移：旧会话、消息和工具记录全部保留，空值由启动升级流程回填。
	for _, item := range []struct {
		table  string
		column string
		ddl    string
	}{
		{table: "sessions", column: "system_prompt_snapshot", ddl: `ALTER TABLE sessions ADD COLUMN system_prompt_snapshot TEXT NOT NULL DEFAULT ''`},
		{table: "sessions", column: "project_prompt_snapshot", ddl: `ALTER TABLE sessions ADD COLUMN project_prompt_snapshot TEXT NOT NULL DEFAULT ''`},
		{table: "sessions", column: "system_prompt_frozen", ddl: `ALTER TABLE sessions ADD COLUMN system_prompt_frozen INTEGER NOT NULL DEFAULT 0`},
		{table: "sessions", column: "project_prompt_frozen", ddl: `ALTER TABLE sessions ADD COLUMN project_prompt_frozen INTEGER NOT NULL DEFAULT 0`},
		{table: "session_messages", column: "usage_json", ddl: `ALTER TABLE session_messages ADD COLUMN usage_json TEXT NOT NULL DEFAULT ''`},
	} {
		exists, err := sqliteColumnExists(db, item.table, item.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec(item.ddl); err != nil {
			return fmt.Errorf("add %s.%s: %w", item.table, item.column, err)
		}
	}
	return nil
}

func backfillMCPAppEventMeta(db *sql.DB) error {
	// 旧事件可能已经把完整 Apps detail 放进独立表，但 meta 里只有 tool 名。
	// 这里只回填渲染入口需要的轻量字段，HTML 继续留在 detail 表按需加载。
	stmts := []string{
		`UPDATE session_message_events
SET meta = json_set(
  CASE WHEN json_valid(meta) THEN meta ELSE '{}' END,
  '$.data.mcp_app',
  json_object(
    'server', COALESCE(json_extract((SELECT d.details_json FROM session_message_event_details d WHERE d.session_id = session_message_events.session_id AND d.event_id = session_message_events.id), '$.data.mcp_app.server'), ''),
    'resource_uri', COALESCE(json_extract((SELECT d.details_json FROM session_message_event_details d WHERE d.session_id = session_message_events.session_id AND d.event_id = session_message_events.id), '$.data.mcp_app.resource_uri'), ''),
    'mime_type', COALESCE(json_extract((SELECT d.details_json FROM session_message_event_details d WHERE d.session_id = session_message_events.session_id AND d.event_id = session_message_events.id), '$.data.mcp_app.mime_type'), '')
  )
)
WHERE json_valid(meta)
  AND json_type(CASE WHEN json_valid(meta) THEN meta ELSE '{}' END, '$.data.mcp_app') IS NULL
  AND EXISTS (
    SELECT 1 FROM session_message_event_details d
    WHERE d.session_id = session_message_events.session_id
      AND d.event_id = session_message_events.id
      AND json_type(d.details_json, '$.data.mcp_app') = 'object'
  )`,
		`UPDATE session_message_events
SET meta = json_set(
  CASE WHEN json_valid(meta) THEN meta ELSE '{}' END,
  '$.data.mcp_app_error',
  json_extract((SELECT d.details_json FROM session_message_event_details d WHERE d.session_id = session_message_events.session_id AND d.event_id = session_message_events.id), '$.data.mcp_app_error')
)
WHERE json_valid(meta)
  AND json_type(CASE WHEN json_valid(meta) THEN meta ELSE '{}' END, '$.data.mcp_app_error') IS NULL
  AND EXISTS (
    SELECT 1 FROM session_message_event_details d
    WHERE d.session_id = session_message_events.session_id
      AND d.event_id = session_message_events.id
      AND json_type(d.details_json, '$.data.mcp_app_error') = 'text'
  )`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("backfill MCP App event meta: %w", err)
		}
	}
	return nil
}

func ensurePinnedEntityColumns(db *sql.DB) error {
	// 置顶是当前项目 schema 的向后兼容字段；旧数据默认保持未置顶。
	for _, item := range []struct {
		table  string
		column string
	}{
		{table: "projects", column: "pinned"},
		{table: "scheduled_tasks", column: "pinned"},
	} {
		exists, err := sqliteColumnExists(db, item.table, item.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE ` + item.table + ` ADD COLUMN ` + item.column + ` INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add %s.%s: %w", item.table, item.column, err)
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
	for _, table := range []string{"sessions", "session_messages", "session_message_parts", "session_message_events", "session_message_event_details", "session_tool_working_set", "mcp_runs", "mcp_confirmations", "chat_jobs", "scheduled_tasks", "scheduled_task_runs", "attachments", "tool_embeddings"} {
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
