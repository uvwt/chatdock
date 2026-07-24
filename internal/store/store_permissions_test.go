package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewStoreCreatesPrivateDataArtifacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}
	dataDir := filepath.Join(t.TempDir(), "chatdock-data")
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	assertPermission(t, dataDir, privateDirMode)
	assertPermission(t, filepath.Join(dataDir, "chatdock.sqlite"), privateFileMode)
}

func assertPermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permission = %04o, want %04o", path, got, want)
	}
}
