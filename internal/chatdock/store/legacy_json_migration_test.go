package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestLegacySessionMigrationFailsWithoutMarkingInvalidDataComplete(t *testing.T) {
	dataDir := t.TempDir()
	sessionsDir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionsDir, "broken.json")
	if err := os.WriteFile(path, []byte(`{"id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dataDir); err == nil || !strings.Contains(err.Error(), "decode legacy session") {
		t.Fatalf("expected invalid legacy session to block migration, got %v", err)
	}

	now := time.Now()
	raw, err := json.Marshal(model.Session{ID: "recovered", Title: "Recovered", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, ok, err := store.GetSession("default", "recovered")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || session.Title != "Recovered" {
		t.Fatalf("repaired legacy session should migrate on retry: ok=%v session=%#v", ok, session)
	}
}
