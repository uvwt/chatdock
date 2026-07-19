package chatdock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestResolveAttachmentStoragePathSupportsCurrentRelativeAndRelocatedUploads(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "uploads", "default", "file.txt")
	if got, err := resolveAttachmentStoragePath(root, current); err != nil || got != current {
		t.Fatalf("current absolute path = %q, %v", got, err)
	}
	if got, err := resolveAttachmentStoragePath(root, filepath.Join("uploads", "default", "file.txt")); err != nil || got != current {
		t.Fatalf("canonical relative path = %q, %v", got, err)
	}

	parent := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.MkdirAll(filepath.Join("data", "uploads", "default"), 0o700); err != nil {
		t.Fatal(err)
	}
	wantDefault := filepath.Join(parent, "data", "uploads", "default", "file.txt")
	if got, err := resolveAttachmentStoragePath("data", filepath.Join("data", "uploads", "default", "file.txt")); err != nil || got != wantDefault {
		t.Fatalf("default relative data path = %q, %v", got, err)
	}

	if runtime.GOOS != "windows" {
		oldHostPath := "/Volumes/OLD/Docker/chatdock/data/uploads/default/file.txt"
		if got, err := resolveAttachmentStoragePath(root, oldHostPath); err != nil || got != current {
			t.Fatalf("relocated host path = %q, %v", got, err)
		}
	}
}

func TestResolveAttachmentStoragePathRejectsExternalAndTraversalPaths(t *testing.T) {
	root := t.TempDir()
	for _, stored := range []string{
		"../secret.txt",
		filepath.Join(t.TempDir(), "secret.txt"),
		filepath.Join("other", "file.txt"),
	} {
		if got, err := resolveAttachmentStoragePath(root, stored); err == nil {
			t.Fatalf("unsafe path %q resolved to %q", stored, got)
		}
	}
}

func TestDownloadResolvesRelocatedHistoricalAttachmentPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("historical production path is POSIX")
	}
	dataDir := t.TempDir()
	actualPath := filepath.Join(dataDir, "uploads", "default", "historical.txt")
	if err := os.MkdirAll(filepath.Dir(actualPath), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("historical attachment")
	if err := os.WriteFile(actualPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	record := model.AttachmentRecord{
		Attachment: model.Attachment{ID: "historical", Name: "historical.txt", MIMEType: "text/plain", Size: int64(len(content)), Status: "stored", CreatedAt: time.Now()},
		Prompt:     "default", StoragePath: "/Volumes/OLD/Docker/chatdock/data/uploads/default/historical.txt",
	}
	if _, err := app.store.SaveAttachment("default", record); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/files/historical", nil)
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != string(content) {
		t.Fatalf("historical download status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestPersistUploadedFileReusesBlobBehindRelocatedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("historical production path is POSIX")
	}
	dataDir := t.TempDir()
	actualPath := filepath.Join(dataDir, "uploads", "default", "existing.txt")
	if err := os.MkdirAll(filepath.Dir(actualPath), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("shared attachment content")
	if err := os.WriteFile(actualPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	sha := hex.EncodeToString(sum[:])
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	record := model.AttachmentRecord{
		Attachment: model.Attachment{ID: "existing", Name: "existing.txt", MIMEType: "text/plain", Size: int64(len(content)), Status: "stored", CreatedAt: time.Now()},
		Prompt:     "default", StoragePath: "/Volumes/OLD/Docker/chatdock/data/uploads/default/existing.txt", SHA256: sha,
	}
	if _, err := app.store.SaveAttachment("default", record); err != nil {
		t.Fatal(err)
	}

	upload, err := app.persistUploadedFile("default", "duplicate", "duplicate.txt", "text/plain", bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if upload.OwnsStorage || !strings.Contains(upload.StoragePath, "/Volumes/OLD/") {
		t.Fatalf("relocated blob was not reused: %#v", upload)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "uploads", "default", "duplicate_duplicate.txt")); !os.IsNotExist(err) {
		t.Fatalf("duplicate upload file was not removed: %v", err)
	}
}
