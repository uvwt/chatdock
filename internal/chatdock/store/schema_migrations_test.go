package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSQLiteSchemaMigrationRollsBackDDLAndRecordTogether(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	migration := sqliteSchemaMigration{
		Version: 999,
		Name:    "rollback_probe",
		Statements: []string{
			`CREATE TABLE migration_probe (id INTEGER PRIMARY KEY)`,
			`INSERT INTO missing_migration_table(id) VALUES(1)`,
		},
	}
	if err := store.runSQLiteSchemaMigration(migration); err == nil {
		t.Fatal("expected migration failure")
	}
	var tableCount, recordCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'migration_probe'`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 999`).Scan(&recordCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 || recordCount != 0 {
		t.Fatalf("partial migration persisted: table=%d record=%d", tableCount, recordCount)
	}
}

func TestSQLiteSchemaMigrationRecordsAlreadyAppliedDDL(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE migration_existing (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	migration := sqliteSchemaMigration{Version: 1000, Name: "existing_table", Statements: []string{`CREATE TABLE migration_existing (id INTEGER PRIMARY KEY)`}}
	if err := store.runSQLiteSchemaMigration(migration); err != nil {
		t.Fatal(err)
	}
	var recordCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1000`).Scan(&recordCount); err != nil {
		t.Fatal(err)
	}
	if recordCount != 1 {
		t.Fatalf("migration record count = %d, want 1", recordCount)
	}
}

func TestSQLiteSchemaMigrationAddsMessageErrorColumnToExistingDatabase(t *testing.T) {
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dataDir, "chatdock.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE session_messages (
		workspace_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		message_index INTEGER NOT NULL,
		id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		reasoning TEXT NOT NULL DEFAULT '',
		attachments_json TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		PRIMARY KEY(workspace_id, session_id, message_index)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hasColumn, err := sqliteColumnExists(store.db, "session_messages", "error_json")
	if err != nil {
		t.Fatal(err)
	}
	if !hasColumn {
		t.Fatal("session_messages.error_json was not added")
	}
	applied, err := store.sqliteSchemaMigrationApplied(7)
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("session_messages_error_json migration was not recorded")
	}
}
