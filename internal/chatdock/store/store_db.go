package store

import (
	"chatdock/internal/chatdock/model"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// SQLite 初始化和旧 JSON 数据迁移放在一起，便于先读懂 Store 启动流程，
// 也避免把迁移细节混进会话/工作空间的业务方法里。

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
	if err := s.migrateSQLiteWorkspaceSchema(); err != nil {
		return err
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (name TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS workspace_kv (workspace_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(workspace_id, key), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS sessions (workspace_id TEXT NOT NULL, id TEXT NOT NULL, json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(workspace_id, id), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_workspace_updated ON sessions(workspace_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS session_messages (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, message_index INTEGER NOT NULL, id TEXT NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL, reasoning TEXT NOT NULL DEFAULT '', attachments_json TEXT NOT NULL DEFAULT '[]', created_at TEXT NOT NULL, PRIMARY KEY(workspace_id, session_id, message_index), FOREIGN KEY(workspace_id, session_id) REFERENCES sessions(workspace_id, id) ON DELETE CASCADE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_messages_id ON session_messages(workspace_id, session_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_session_messages_session ON session_messages(workspace_id, session_id, message_index)`,
		`CREATE TABLE IF NOT EXISTS session_message_parts (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, message_index INTEGER NOT NULL, part_index INTEGER NOT NULL, kind TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', event_id TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, session_id, message_index, part_index), FOREIGN KEY(workspace_id, session_id, message_index) REFERENCES session_messages(workspace_id, session_id, message_index) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS session_message_events (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, message_index INTEGER NOT NULL, event_index INTEGER NOT NULL, id TEXT NOT NULL, kind TEXT NOT NULL, phase TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', text TEXT NOT NULL DEFAULT '', meta TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, session_id, message_index, event_index), FOREIGN KEY(workspace_id, session_id, message_index) REFERENCES session_messages(workspace_id, session_id, message_index) ON DELETE CASCADE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_message_events_id ON session_message_events(workspace_id, session_id, id)`,
		`CREATE TABLE IF NOT EXISTS session_message_event_details (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, event_id TEXT NOT NULL, details_json TEXT NOT NULL, details_bytes INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, PRIMARY KEY(workspace_id, session_id, event_id), FOREIGN KEY(workspace_id, session_id) REFERENCES sessions(workspace_id, id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_session_event_details_bytes ON session_message_event_details(details_bytes DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_runs (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, title TEXT NOT NULL, status TEXT NOT NULL, summary TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, event_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_workspace_updated ON mcp_runs(workspace_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_session_updated ON mcp_runs(session_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_run_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, server TEXT NOT NULL, tool TEXT NOT NULL, action TEXT NOT NULL, summary TEXT NOT NULL, arguments_json TEXT NOT NULL, result_json TEXT NOT NULL, error TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(run_id) REFERENCES mcp_runs(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS mcp_confirmations (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL, arguments_json TEXT NOT NULL DEFAULT '{}', status TEXT NOT NULL, requested_at TEXT NOT NULL, resolved_at TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '', FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_confirmations_workspace_status_requested ON mcp_confirmations(workspace_id, status, requested_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_run_seq ON mcp_run_events(run_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_tool_created ON mcp_run_events(tool, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS chat_jobs (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, request_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, answer TEXT NOT NULL, reasoning TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_jobs_workspace_session_updated ON chat_jobs(workspace_id, session_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_jobs_status_updated ON chat_jobs(status, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS chat_job_events (job_id TEXT NOT NULL, seq INTEGER NOT NULL, event TEXT NOT NULL, data_json TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(job_id, seq), FOREIGN KEY(job_id) REFERENCES chat_jobs(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_job_events_job_event_seq ON chat_job_events(job_id, event, seq)`,
		`CREATE TABLE IF NOT EXISTS scheduled_tasks (workspace_id TEXT NOT NULL, id TEXT NOT NULL, title TEXT NOT NULL, task_prompt TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, running INTEGER NOT NULL DEFAULT 0, schedule_type TEXT NOT NULL, run_at TEXT NOT NULL DEFAULT '', time_of_day TEXT NOT NULL DEFAULT '', interval_minutes INTEGER NOT NULL DEFAULT 0, context_mode TEXT NOT NULL DEFAULT 'stateless', next_run_at TEXT NOT NULL DEFAULT '', last_run_at TEXT NOT NULL DEFAULT '', last_status TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '', session_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(workspace_id, id), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_due ON scheduled_tasks(enabled, running, next_run_at)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_workspace_updated ON scheduled_tasks(workspace_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS scheduled_task_runs (workspace_id TEXT NOT NULL, id TEXT NOT NULL, task_id TEXT NOT NULL, task_title TEXT NOT NULL, task_prompt TEXT NOT NULL, output TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', manual INTEGER NOT NULL DEFAULT 0, session_id TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL, finished_at TEXT NOT NULL DEFAULT '', duration_ms INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(workspace_id, id), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_task_started ON scheduled_task_runs(workspace_id, task_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_started ON scheduled_task_runs(started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS attachment_blobs (sha256 TEXT PRIMARY KEY, storage_path TEXT NOT NULL, size INTEGER NOT NULL, mime_type TEXT NOT NULL, ref_count INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_attachment_blobs_created ON attachment_blobs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS attachments (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, message_id TEXT NOT NULL, filename TEXT NOT NULL, mime_type TEXT NOT NULL, size INTEGER NOT NULL, storage_path TEXT NOT NULL, sha256 TEXT NOT NULL, text_content TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_workspace_session ON attachments(workspace_id, session_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS tool_embeddings (workspace_id TEXT NOT NULL, full_name TEXT NOT NULL, source_hash TEXT NOT NULL, embedding_model TEXT NOT NULL, embedding_json TEXT NOT NULL, embedding_blob BLOB NOT NULL DEFAULT x'', indexed_at TEXT NOT NULL, PRIMARY KEY(workspace_id, full_name, embedding_model), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_embeddings_workspace_model ON tool_embeddings(workspace_id, embedding_model)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return s.ensureSQLiteSchemaUpdates()
}

var workspaceScopedLegacyTables = []string{
	"workspace_kv",
	"sessions",
	"session_messages",
	"session_message_parts",
	"session_message_events",
	"session_message_event_details",
	"mcp_runs",
	"chat_jobs",
	"scheduled_tasks",
	"scheduled_task_runs",
	"attachments",
	"tool_embeddings",
}

var legacyWorkspaceIndexNames = []string{
	"idx_sessions_prompt_updated",
	"idx_mcp_runs_prompt_updated",
	"idx_chat_jobs_prompt_session_updated",
	"idx_scheduled_tasks_prompt_updated",
	"idx_attachments_prompt_session",
	"idx_tool_embeddings_prompt_model",
}

func (s *Store) migrateSQLiteWorkspaceSchema() error {
	// 只兼容旧数据迁移，不兼容旧 schema：启动时把历史 prompt 表/列原地改成 workspace 命名，
	// 后续业务 SQL 只读写 workspace_id，不再保留 prompt 双读或 fallback。
	if _, err := s.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer s.db.Exec(`PRAGMA foreign_keys = ON`)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, name := range legacyWorkspaceIndexNames {
		if _, err := tx.Exec(`DROP INDEX IF EXISTS ` + name); err != nil {
			return err
		}
	}
	if exists, err := sqliteTableExists(tx, "prompts"); err != nil {
		return err
	} else if exists {
		if existsNew, err := sqliteTableExists(tx, "workspaces"); err != nil {
			return err
		} else if !existsNew {
			if _, err := tx.Exec(`ALTER TABLE prompts RENAME TO workspaces`); err != nil {
				return err
			}
		}
	}
	if exists, err := sqliteTableExists(tx, "prompt_kv"); err != nil {
		return err
	} else if exists {
		if existsNew, err := sqliteTableExists(tx, "workspace_kv"); err != nil {
			return err
		} else if !existsNew {
			if _, err := tx.Exec(`ALTER TABLE prompt_kv RENAME TO workspace_kv`); err != nil {
				return err
			}
		}
	}
	for _, table := range workspaceScopedLegacyTables {
		exists, err := sqliteTableExists(tx, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		hasOld, err := sqliteColumnExists(tx, table, "prompt")
		if err != nil {
			return err
		}
		hasNew, err := sqliteColumnExists(tx, table, "workspace_id")
		if err != nil {
			return err
		}
		if hasOld && !hasNew {
			if _, err := tx.Exec(`ALTER TABLE ` + table + ` RENAME COLUMN prompt TO workspace_id`); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
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

func (s *Store) migrateLegacyData() error {
	migrated, err := s.metaValue("json_migrated")
	if err != nil {
		return err
	}
	if migrated == "1" {
		return s.ensureWorkspaceLocked(defaultWorkspaceID)
	}

	if err := s.ensureWorkspaceLocked(defaultWorkspaceID); err != nil {
		return err
	}
	legacyRoot := filepath.Join(s.dataDir, "prompts")
	if entries, err := os.ReadDir(legacyRoot); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				if err := s.importPromptDir(entry.Name(), filepath.Join(legacyRoot, entry.Name())); err != nil {
					return err
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.importLegacyDefaultFiles(); err != nil {
		return err
	}
	return s.setMetaValue("json_migrated", "1")
}

func (s *Store) importLegacyDefaultFiles() error {
	if _, ok, err := s.getWorkspaceRawLocked(defaultWorkspaceID, "config"); err != nil {
		return err
	} else if !ok {
		legacyConfig := filepath.Join(s.dataDir, "config.json")
		if raw, err := os.ReadFile(legacyConfig); err == nil {
			if err := s.setWorkspaceRawLocked(defaultWorkspaceID, "config", string(raw)); err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := s.setWorkspaceJSONLocked(defaultWorkspaceID, "config", model.DefaultModelConfig()); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	legacySessions := filepath.Join(s.dataDir, "sessions")
	if entries, err := os.ReadDir(legacySessions); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if err := s.importSessionFile(defaultWorkspaceID, filepath.Join(legacySessions, entry.Name())); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) importPromptDir(name string, dir string) error {
	name, err := normalizeWorkspaceID(name)
	if err != nil {
		return nil
	}
	if err := s.ensureWorkspaceLocked(name); err != nil {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		if err := s.setWorkspaceRawLocked(name, "config", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "mcp.json")); err == nil {
		if err := s.setWorkspaceRawLocked(name, "mcp", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "scheduled_tasks.json")); err == nil {
		if err := s.setWorkspaceRawLocked(name, "scheduled_tasks", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sessionsDir := filepath.Join(dir, "sessions")
	if entries, err := os.ReadDir(sessionsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if err := s.importSessionFile(name, filepath.Join(sessionsDir, entry.Name())); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) importSessionFile(prompt string, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var session model.Session
	if err := json.Unmarshal(raw, &session); err != nil || session.ID == "" {
		return nil
	}
	return s.saveSessionForWorkspaceLocked(prompt, &session)
}

func (s *Store) metaValue(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) setMetaValue(key string, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
