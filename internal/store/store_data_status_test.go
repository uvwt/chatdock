package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDataStatusReportsLatestBackup(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldBackup := filepath.Join(backupDir, "chatdock.sqlite.20260620-010000.bak")
	newBackup := filepath.Join(backupDir, "chatdock.sqlite.20260620-020000.bak")
	ignoredEnvBackup := filepath.Join(backupDir, ".env.20260620-030000.bak")
	ignoredComposeBackup := filepath.Join(backupDir, "compose.yaml.20260620-030000.bak")
	if err := os.WriteFile(oldBackup, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBackup, []byte("new-backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignoredEnvBackup, []byte("env"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignoredComposeBackup, []byte("compose"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-3 * time.Hour)
	newTime := time.Now().Add(-2 * time.Hour)
	ignoredTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldBackup, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newBackup, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(ignoredEnvBackup, ignoredTime, ignoredTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(ignoredComposeBackup, ignoredTime, ignoredTime); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.DataStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.BackupDir != backupDir || status.BackupCount != 2 || status.LatestBackupPath != newBackup || status.LatestBackupSizeBytes != int64(len("new-backup")) {
		t.Fatalf("unexpected backup status: %#v", status)
	}
	if len(status.BackupCheckedDirs) != 2 || status.BackupCheckedDirs[0] != backupDir || status.BackupCheckedDirs[1] != filepath.Join(dataDir, "backups") {
		t.Fatalf("unexpected backup checked dirs: %#v", status.BackupCheckedDirs)
	}
	if !status.BackupHealthy || status.BackupWarning != "" || status.LatestBackupAgeSeconds <= 0 {
		t.Fatalf("unexpected backup health status: %#v", status)
	}
	if len(status.Backups) != 2 || status.Backups[0].Path != newBackup || status.Backups[0].Name != "chatdock.sqlite.20260620-020000.bak" || status.Backups[1].Path != oldBackup {
		t.Fatalf("unexpected backup list: %#v", status.Backups)
	}
	if status.Backups[0].AgeSeconds <= 0 || status.Backups[1].AgeSeconds <= status.Backups[0].AgeSeconds {
		t.Fatalf("unexpected backup age list: %#v", status.Backups)
	}
	for _, backup := range status.Backups {
		if strings.HasPrefix(backup.Name, ".env") || strings.HasPrefix(backup.Name, "compose.yaml") {
			t.Fatalf("non-database backup should not be reported: %#v", status.Backups)
		}
	}
}

func TestDataStatusWarnsWhenBackupIsMissingOrStale(t *testing.T) {
	missingStore, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing, err := missingStore.DataStatus()
	if err != nil {
		t.Fatal(err)
	}
	if missing.BackupHealthy || missing.BackupWarning != "未检测到数据库备份" {
		t.Fatalf("missing backup should warn: %#v", missing)
	}

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleBackup := filepath.Join(backupDir, "chatdock.sqlite.20260617-010000.bak")
	if err := os.WriteFile(staleBackup, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(staleBackup, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}
	staleStore, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := staleStore.DataStatus()
	if err != nil {
		t.Fatal(err)
	}
	if stale.BackupHealthy || stale.BackupWarning != "最近数据库备份超过 48 小时" || stale.LatestBackupAgeSeconds < int64(70*time.Hour/time.Second) {
		t.Fatalf("stale backup should warn: %#v", stale)
	}
}
