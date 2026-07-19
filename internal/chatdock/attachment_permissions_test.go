package chatdock

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestPersistUploadedFileUsesPrivatePermissionsAndNeverOverwrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}
	dataDir := t.TempDir()
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})

	upload, err := app.persistUploadedFile("default", "fixed-id", "notes.txt", "text/plain", bytes.NewBufferString("first"))
	if err != nil {
		t.Fatal(err)
	}
	uploadDir := filepath.Dir(upload.StoragePath)
	assertPathPermission(t, uploadDir, uploadDirMode)
	assertPathPermission(t, upload.StoragePath, uploadFileMode)

	if _, err := app.persistUploadedFile("default", "fixed-id", "notes.txt", "text/plain", bytes.NewBufferString("second")); err == nil {
		t.Fatal("duplicate upload path should not overwrite the existing file")
	}
	raw, err := os.ReadFile(upload.StoragePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "first" {
		t.Fatalf("existing upload was overwritten: %q", raw)
	}
}

func assertPathPermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permission = %04o, want %04o", path, got, want)
	}
}
