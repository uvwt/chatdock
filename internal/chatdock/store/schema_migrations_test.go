package store

import "testing"

func TestSQLiteSchemaMigrationsRecorded(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != len(sqliteSchemaMigrations) {
		t.Fatalf("schema migration count = %d, want %d", count, len(sqliteSchemaMigrations))
	}
}
