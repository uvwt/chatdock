package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLegacyJSONFileEnforcesLimitWithoutReadingWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if raw, err := readLegacyJSONFile(path, int64(len(`{"ok":true}`))); err != nil || string(raw) != `{"ok":true}` {
		t.Fatalf("exact-limit read = %q, %v", raw, err)
	}
	if err := os.Truncate(path, 33); err != nil {
		t.Fatal(err)
	}
	if _, err := readLegacyJSONFile(path, 32); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("oversized legacy JSON error = %v", err)
	}
}

func TestReadLegacyJSONFilePreservesMissingFileIdentity(t *testing.T) {
	_, err := readLegacyJSONFile(filepath.Join(t.TempDir(), "missing.json"), 32)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing legacy JSON error = %v", err)
	}
}

func TestNewStoreRejectsOversizedLegacyConfigBeforeMigration(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "config.json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxLegacyConfigJSONBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dataDir)
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "config.json exceeds") {
		t.Fatalf("oversized legacy config error = %v", err)
	}
}
