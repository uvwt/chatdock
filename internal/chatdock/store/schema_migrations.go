package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type sqliteSchemaMigration struct {
	Version    int
	Name       string
	Statements []string
	Apply      func(*sql.Tx) error
}

var sqliteSchemaMigrations = []sqliteSchemaMigration{
	{Version: 1, Name: "chat_jobs_request_id", Statements: []string{`ALTER TABLE chat_jobs ADD COLUMN request_id TEXT NOT NULL DEFAULT ''`}},
	{Version: 2, Name: "sessions_title", Statements: []string{`ALTER TABLE sessions ADD COLUMN title TEXT NOT NULL DEFAULT ''`}},
	{Version: 3, Name: "sessions_pinned", Statements: []string{`ALTER TABLE sessions ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0`}},
	{Version: 4, Name: "sessions_provider_id", Statements: []string{`ALTER TABLE sessions ADD COLUMN provider_id TEXT NOT NULL DEFAULT ''`}},
	{Version: 5, Name: "sessions_model", Statements: []string{`ALTER TABLE sessions ADD COLUMN model TEXT NOT NULL DEFAULT ''`}},
	{Version: 6, Name: "tool_embeddings_embedding_blob", Statements: []string{`ALTER TABLE tool_embeddings ADD COLUMN embedding_blob BLOB NOT NULL DEFAULT x''`}},
	{Version: 7, Name: "session_messages_error_json", Statements: []string{`ALTER TABLE session_messages ADD COLUMN error_json TEXT NOT NULL DEFAULT ''`}},
	{Version: 8, Name: "scheduled_tasks_cron", Apply: migrateScheduledTasksCronSchema},
	{Version: 9, Name: "sessions_page_indexes", Statements: []string{
		`CREATE INDEX IF NOT EXISTS idx_sessions_workspace_pinned_updated ON sessions(workspace_id, pinned DESC, updated_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_workspace_session ON scheduled_tasks(workspace_id, session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_workspace_session ON scheduled_task_runs(workspace_id, session_id)`,
	}},
}

func (s *Store) ensureSQLiteSchemaUpdates() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	for _, migration := range sqliteSchemaMigrations {
		applied, err := s.sqliteSchemaMigrationApplied(migration.Version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := s.runSQLiteSchemaMigration(migration); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) sqliteSchemaMigrationApplied(version int) (bool, error) {
	var existing int
	err := s.db.QueryRow(`SELECT version FROM schema_migrations WHERE version = ?`, version).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) runSQLiteSchemaMigration(migration sqliteSchemaMigration) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range migration.Statements {
		if _, err := tx.Exec(stmt); err != nil {
			if isSQLiteSchemaAlreadyAppliedError(err) {
				continue
			}
			return fmt.Errorf("apply sqlite migration %03d %s: %w", migration.Version, migration.Name, err)
		}
	}
	if migration.Apply != nil {
		if err := migration.Apply(tx); err != nil {
			return fmt.Errorf("apply sqlite migration %03d %s: %w", migration.Version, migration.Name, err)
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?)`, migration.Version, migration.Name, formatDBTime(time.Now())); err != nil {
		return fmt.Errorf("record sqlite migration %03d %s: %w", migration.Version, migration.Name, err)
	}
	return tx.Commit()
}

func isSQLiteSchemaAlreadyAppliedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column name") || strings.Contains(message, "already exists")
}
