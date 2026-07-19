package chatdock

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestNewAppNormalizesDataDirBeforeCreatingStoreAndUploads(t *testing.T) {
	root := t.TempDir()
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: "  ", WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})
	if app.cfg.DataDir != "data" {
		t.Fatalf("normalized data dir = %q", app.cfg.DataDir)
	}
	status, err := app.store.DataStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.DataDir != app.cfg.DataDir {
		t.Fatalf("store data dir = %q, app data dir = %q", status.DataDir, app.cfg.DataDir)
	}

	upload, err := app.persistUploadedFile("default", "normalized", "notes.txt", "text/plain", bytes.NewBufferString("content"))
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := filepath.Join(app.cfg.DataDir, "uploads", "default") + string(filepath.Separator)
	if !strings.HasPrefix(upload.StoragePath, wantPrefix) {
		t.Fatalf("upload path = %q, want prefix %q", upload.StoragePath, wantPrefix)
	}
}

func TestNewAppTrimsExplicitDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "custom-data")
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: "  " + dataDir + "  ", WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})
	if app.cfg.DataDir != dataDir {
		t.Fatalf("normalized data dir = %q, want %q", app.cfg.DataDir, dataDir)
	}
}
