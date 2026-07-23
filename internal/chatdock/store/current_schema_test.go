package store

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFreshSchemaUsesGlobalSettingsAndProjects(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	columns := tableColumnNames(t, store.db, "sessions")
	wantColumns := []string{"id", "project_id", "title", "pinned", "provider_id", "model", "created_at", "updated_at"}
	if !reflect.DeepEqual(columns, wantColumns) {
		t.Fatalf("sessions columns = %#v, want %#v", columns, wantColumns)
	}
	for _, table := range []string{"global_settings", "projects", "sessions", "scheduled_tasks", "attachments", "tool_embeddings"} {
		exists, err := sqliteTableExists(store.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing current schema table %s", table)
		}
	}
	for _, table := range []string{"schema_migrations", "workspaces", "workspace_kv", "prompts", "prompt_kv"} {
		exists, err := sqliteTableExists(store.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("unexpected legacy or runtime migration table %s", table)
		}
	}
	assertProjectForeignKey(t, store.db)
	var quickCheck string
	if err := store.db.QueryRow(`PRAGMA quick_check`).Scan(&quickCheck); err != nil {
		t.Fatal(err)
	}
	if quickCheck != "ok" {
		t.Fatalf("quick_check = %q", quickCheck)
	}
}

func TestNewStoreRejectsLegacyWorkspaceSchema(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{name: "workspace table", sql: `CREATE TABLE workspaces (name TEXT PRIMARY KEY)`},
		{name: "workspace column", sql: `CREATE TABLE sessions (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataDir := t.TempDir()
			db, err := sql.Open("sqlite3", filepath.Join(dataDir, "chatdock.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tt.sql); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			store, err := NewStore(dataDir)
			if store != nil {
				_ = store.Close()
				t.Fatal("legacy schema unexpectedly opened")
			}
			if err == nil || !strings.Contains(err.Error(), "external one-time migration") {
				t.Fatalf("legacy schema error = %v", err)
			}
		})
	}
}

func tableColumnNames(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func assertProjectForeignKey(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_list(sessions)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatal(err)
		}
		if table == "projects" && from == "project_id" && to == "id" && onDelete == "SET NULL" {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("sessions.project_id SET NULL foreign key is missing")
}
