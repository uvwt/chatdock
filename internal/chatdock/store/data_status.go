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
	active := s.activePrompt
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return DataStatus{}, err
	}
	var sessionCount int
	for _, prompt := range prompts {
		sessionCount += prompt.Count
	}
	info, err := os.Stat(dbPath)
	if err != nil && !os.IsNotExist(err) {
		return DataStatus{}, err
	}
	_, walErr := os.Stat(dbPath + "-wal")
	_, shmErr := os.Stat(dbPath + "-shm")
	status := DataStatus{
		DataDir:         dataDir,
		DatabasePath:    dbPath,
		DatabaseExists:  info != nil,
		WALEnabled:      walErr == nil,
		SHMExists:       shmErr == nil,
		ActiveWorkspace: active,
		WorkspaceCount:  len(prompts),
		SessionCount:    sessionCount,
	}
	if info != nil {
		status.DatabaseSizeBytes = info.Size()
		// quick_check 只读且开销很小，适合在数据状态页暴露 SQLite 文件健康度。
		var quickCheck string
		if err := s.db.QueryRow("PRAGMA quick_check").Scan(&quickCheck); err != nil {
			status.DatabaseWarning = "SQLite quick_check 执行失败"
		} else {
			status.DatabaseCheck = quickCheck
			status.DatabaseHealthy = quickCheck == "ok"
			if !status.DatabaseHealthy {
				status.DatabaseWarning = "SQLite quick_check 未通过"
			}
		}
	}
	backupDirs := []string{filepath.Join(filepath.Dir(dataDir), "backups"), filepath.Join(dataDir, "backups")}
	seenBackupDir := map[string]bool{}
	for _, dir := range backupDirs {
		if dir == "" || seenBackupDir[dir] {
			continue
		}
		seenBackupDir[dir] = true
		status.BackupCheckedDirs = append(status.BackupCheckedDirs, dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return DataStatus{}, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !isDatabaseBackupFile(name) {
				continue
			}
			fileInfo, err := entry.Info()
			if err != nil {
				return DataStatus{}, err
			}
			status.BackupDir = dir
			status.BackupCount++
			status.Backups = append(status.Backups, BackupInfo{
				Name:       name,
				Path:       filepath.Join(dir, name),
				SizeBytes:  fileInfo.Size(),
				UpdatedAt:  fileInfo.ModTime(),
				AgeSeconds: int64(time.Since(fileInfo.ModTime()).Seconds()),
			})
		}
	}
	sort.Slice(status.Backups, func(i, j int) bool {
		return status.Backups[i].UpdatedAt.After(status.Backups[j].UpdatedAt)
	})
	// 备份健康状态用于配置中心自助排障：48 小时内有数据库备份才视为健康。
	if len(status.Backups) > 0 {
		latest := status.Backups[0]
		status.LatestBackupAt = latest.UpdatedAt
		status.LatestBackupPath = latest.Path
		status.LatestBackupSizeBytes = latest.SizeBytes
		status.LatestBackupAgeSeconds = int64(time.Since(latest.UpdatedAt).Seconds())
		status.BackupHealthy = status.LatestBackupAgeSeconds <= int64(48*time.Hour/time.Second)
		if !status.BackupHealthy {
			status.BackupWarning = "最近数据库备份超过 48 小时"
		}
	} else {
		status.BackupWarning = "未检测到数据库备份"
	}
	if len(status.Backups) > 5 {
		status.Backups = status.Backups[:5]
	}
	return status, nil
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
