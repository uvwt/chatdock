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
	stmts := []string{
		`PRAGMA journal_mode = DELETE`,
		`PRAGMA synchronous = FULL`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS prompts (name TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS prompt_kv (prompt TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(prompt, key), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS sessions (prompt TEXT NOT NULL, id TEXT NOT NULL, json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(prompt, id), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_prompt_updated ON sessions(prompt, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_runs (prompt TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, title TEXT NOT NULL, status TEXT NOT NULL, summary TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, event_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_prompt_updated ON mcp_runs(prompt, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_session_updated ON mcp_runs(session_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_run_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, server TEXT NOT NULL, tool TEXT NOT NULL, action TEXT NOT NULL, summary TEXT NOT NULL, arguments_json TEXT NOT NULL, result_json TEXT NOT NULL, error TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(run_id) REFERENCES mcp_runs(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_run_seq ON mcp_run_events(run_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_tool_created ON mcp_run_events(tool, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS chat_jobs (prompt TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, request_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, answer TEXT NOT NULL, reasoning TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_jobs_prompt_session_updated ON chat_jobs(prompt, session_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_jobs_status_updated ON chat_jobs(status, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS chat_job_events (job_id TEXT NOT NULL, seq INTEGER NOT NULL, event TEXT NOT NULL, data_json TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(job_id, seq), FOREIGN KEY(job_id) REFERENCES chat_jobs(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS attachments (prompt TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, message_id TEXT NOT NULL, filename TEXT NOT NULL, mime_type TEXT NOT NULL, size INTEGER NOT NULL, storage_path TEXT NOT NULL, sha256 TEXT NOT NULL, text_content TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_prompt_session ON attachments(prompt, session_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS tool_embeddings (prompt TEXT NOT NULL, full_name TEXT NOT NULL, source_hash TEXT NOT NULL, embedding_model TEXT NOT NULL, embedding_json TEXT NOT NULL, indexed_at TEXT NOT NULL, PRIMARY KEY(prompt, full_name, embedding_model), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_embeddings_prompt_model ON tool_embeddings(prompt, embedding_model)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return s.ensureSQLiteSchemaUpdates()
}

func (s *Store) ensureSQLiteSchemaUpdates() error {
	stmts := []string{
		`ALTER TABLE chat_jobs ADD COLUMN request_id TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyData() error {
	migrated, err := s.metaValue("json_migrated")
	if err != nil {
		return err
	}
	if migrated == "1" {
		return s.ensurePromptLocked(defaultPromptName)
	}

	if err := s.ensurePromptLocked(defaultPromptName); err != nil {
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
	if _, ok, err := s.getPromptRawLocked(defaultPromptName, "config"); err != nil {
		return err
	} else if !ok {
		legacyConfig := filepath.Join(s.dataDir, "config.json")
		if raw, err := os.ReadFile(legacyConfig); err == nil {
			if err := s.setPromptRawLocked(defaultPromptName, "config", string(raw)); err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := s.setPromptJSONLocked(defaultPromptName, "config", model.DefaultModelConfig()); err != nil {
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
			if err := s.importSessionFile(defaultPromptName, filepath.Join(legacySessions, entry.Name())); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) importPromptDir(name string, dir string) error {
	name, err := normalizePromptName(name)
	if err != nil {
		return nil
	}
	if err := s.ensurePromptLocked(name); err != nil {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		if err := s.setPromptRawLocked(name, "config", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "mcp.json")); err == nil {
		if err := s.setPromptRawLocked(name, "mcp", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "skills.json")); err == nil {
		if err := s.setPromptRawLocked(name, "skills", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "scheduled_tasks.json")); err == nil {
		if err := s.setPromptRawLocked(name, "scheduled_tasks", string(raw)); err != nil {
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
	return s.saveSessionForPromptLocked(prompt, &session)
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
