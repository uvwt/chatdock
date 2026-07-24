package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) DataStatus() (DataStatus, error) {
	s.mu.RLock()
	dataDir := s.dataDir
	dbPath := s.dbPath
	projectCount, err := projectCountWith(s.db)
	if err != nil {
		s.mu.RUnlock()
		return DataStatus{}, err
	}
	var sessionCount int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessionCount)
	s.mu.RUnlock()
	if err != nil {
		return DataStatus{}, err
	}
	status := DataStatus{
		DataDir:      dataDir,
		DatabasePath: dbPath,
		ProjectCount: projectCount,
		SessionCount: sessionCount,
	}
	if err := s.populateDatabaseHealth(&status); err != nil {
		return DataStatus{}, err
	}
	now := time.Now()
	backups, checkedDirs, backupDir, err := scanDatabaseBackups(dataDir, now)
	if err != nil {
		return DataStatus{}, err
	}
	status.Backups = backups
	status.BackupCheckedDirs = checkedDirs
	status.BackupDir = backupDir
	finalizeBackupStatus(&status, now)
	return status, nil
}

func (s *Store) populateDatabaseHealth(status *DataStatus) error {
	info, err := os.Stat(status.DatabasePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	_, walErr := os.Stat(status.DatabasePath + "-wal")
	_, shmErr := os.Stat(status.DatabasePath + "-shm")
	status.DatabaseExists = info != nil
	status.WALEnabled = walErr == nil
	status.SHMExists = shmErr == nil
	if info == nil {
		return nil
	}
	status.DatabaseSizeBytes = info.Size()
	var quickCheck string
	if err := s.db.QueryRow("PRAGMA quick_check").Scan(&quickCheck); err != nil {
		status.DatabaseWarning = "SQLite quick_check 执行失败"
		return nil
	}
	status.DatabaseCheck = quickCheck
	status.DatabaseHealthy = quickCheck == "ok"
	if !status.DatabaseHealthy {
		status.DatabaseWarning = "SQLite quick_check 未通过"
	}
	return nil
}

func scanDatabaseBackups(dataDir string, now time.Time) ([]BackupInfo, []string, string, error) {
	backupDirs := []string{filepath.Join(filepath.Dir(dataDir), "backups"), filepath.Join(dataDir, "backups")}
	seen := map[string]bool{}
	backups := []BackupInfo{}
	checkedDirs := []string{}
	backupDir := ""
	for _, dir := range backupDirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		checkedDirs = append(checkedDirs, dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, "", err
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !isDatabaseBackupFile(name) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, nil, "", err
			}
			backupDir = dir
			backups = append(backups, BackupInfo{Name: name, Path: filepath.Join(dir, name), SizeBytes: info.Size(), UpdatedAt: info.ModTime(), AgeSeconds: int64(now.Sub(info.ModTime()).Seconds())})
		}
	}
	return backups, checkedDirs, backupDir, nil
}

func finalizeBackupStatus(status *DataStatus, now time.Time) {
	status.BackupCount = len(status.Backups)
	sort.Slice(status.Backups, func(i, j int) bool { return status.Backups[i].UpdatedAt.After(status.Backups[j].UpdatedAt) })
	if len(status.Backups) == 0 {
		status.BackupWarning = "未检测到数据库备份"
		return
	}
	latest := status.Backups[0]
	status.LatestBackupAt = latest.UpdatedAt
	status.LatestBackupPath = latest.Path
	status.LatestBackupSizeBytes = latest.SizeBytes
	status.LatestBackupAgeSeconds = int64(now.Sub(latest.UpdatedAt).Seconds())
	status.BackupHealthy = status.LatestBackupAgeSeconds <= int64(48*time.Hour/time.Second)
	if !status.BackupHealthy {
		status.BackupWarning = "最近数据库备份超过 48 小时"
	}
	if len(status.Backups) > 5 {
		status.Backups = status.Backups[:5]
	}
}

func isDatabaseBackupFile(name string) bool {
	lowerName := strings.ToLower(name)
	for _, marker := range []string{".sqlite", ".sqlite3", ".db", ".db3"} {
		if strings.HasSuffix(lowerName, marker) || strings.Contains(lowerName, marker+".") || strings.Contains(lowerName, marker+"-") || strings.Contains(lowerName, marker+"_") {
			return true
		}
	}
	return false
}
