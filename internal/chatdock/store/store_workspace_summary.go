package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

// 工作空间本身只作为显式请求边界存在。Store 不再保存“当前工作空间”，
// 每个读写入口都必须由请求层传入 workspaceID，避免并发请求靠全局缓存互相串扰。

func (s *Store) RequireWorkspace(workspaceID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, err := s.requireWorkspaceLocked(workspaceID)
	return err
}

func (s *Store) requireWorkspaceLocked(workspaceID string) (string, error) {
	name, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return "", err
	}
	exists, err := s.workspaceExistsLocked(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("workspace not found: %s", name)
	}
	return name, nil
}

func (s *Store) ensureWorkspaceDefaultsLocked(name string) error {
	name, err := normalizeWorkspaceID(name)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO workspaces(name, created_at, updated_at) VALUES(?, ?, ?)`, name, formatDBTime(now), formatDBTime(now)); err != nil {
		return err
	}
	if raw, ok, err := getWorkspaceRawWith(tx, name, "config"); err != nil {
		return err
	} else if !ok || strings.TrimSpace(raw) == "" {
		if err := setWorkspaceJSONWith(tx, name, "config", model.DefaultModelConfig(), now); err != nil {
			return err
		}
	}
	if raw, ok, err := getWorkspaceRawWith(tx, name, "mcp"); err != nil {
		return err
	} else if !ok || strings.TrimSpace(raw) == "" {
		if err := setWorkspaceRawWith(tx, name, "mcp", DefaultMCPConfig(), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListWorkspaceSummaries(active string) (model.WorkspaceListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaces, err := listWorkspaceSummariesWith(s.db, active)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: activeWorkspaceFromSummaries(workspaces), Workspaces: workspaces}, nil
}

func (s *Store) CreateWorkspace(input model.CreateWorkspaceRequest) (WorkspaceResponse, error) {
	name, err := normalizeWorkspaceID(input.Name)
	if err != nil {
		return WorkspaceResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.workspaceExistsLocked(name)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	if exists {
		return WorkspaceResponse{}, fmt.Errorf("workspace already exists: %s", name)
	}

	cfg, err := s.modelConfigForWorkspaceLocked(defaultWorkspaceID)
	if err != nil {
		return WorkspaceResponse{}, fmt.Errorf("load default workspace config: %w", err)
	}
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = model.DefaultModelConfig().SystemPrompt
	}
	cfg = model.NormalizeModelConfig(cfg)

	tx, err := s.db.Begin()
	if err != nil {
		return WorkspaceResponse{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	if err := insertWorkspaceRecordsTx(tx, name, now, cfg); err != nil {
		return WorkspaceResponse{}, err
	}
	result, err := listWorkspacesWith(tx, name)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkspaceResponse{}, err
	}
	return result, nil
}

func insertWorkspaceRecordsTx(tx *sql.Tx, name string, now time.Time, cfg model.ModelConfig) error {
	configRaw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspace config: %w", err)
	}
	timestamp := formatDBTime(now)
	if _, err := tx.Exec(`INSERT INTO workspaces(name, created_at, updated_at) VALUES(?, ?, ?)`, name, timestamp, timestamp); err != nil {
		return err
	}
	for _, item := range []struct {
		key   string
		value string
	}{
		{key: "config", value: string(configRaw) + "\n"},
		{key: "mcp", value: DefaultMCPConfig()},
	} {
		if _, err := tx.Exec(`INSERT INTO workspace_kv(workspace_id, key, value, updated_at) VALUES(?, ?, ?, ?)`, name, item.key, item.value, timestamp); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteWorkspace(input model.WorkspaceIDRequest) (WorkspaceResponse, error) {
	name, err := normalizeWorkspaceID(input.Name)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	if name == defaultWorkspaceID {
		return WorkspaceResponse{}, fmt.Errorf("default workspace cannot be deleted")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return WorkspaceResponse{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var workspaceCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE name = ?`, name).Scan(&workspaceCount); err != nil {
		return WorkspaceResponse{}, err
	}
	if workspaceCount == 0 {
		return WorkspaceResponse{}, fmt.Errorf("workspace not found: %s", name)
	}
	names, err := listWorkspaceIDsWith(tx)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	if len(names) <= 1 {
		return WorkspaceResponse{}, fmt.Errorf("last workspace cannot be deleted")
	}
	chatJobs, scheduledTasks, err := workspaceRunningWorkWith(tx, name)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	if chatJobs > 0 || scheduledTasks > 0 {
		return WorkspaceResponse{}, fmt.Errorf("workspace has running work: chat_jobs=%d scheduled_tasks=%d", chatJobs, scheduledTasks)
	}
	// workspace_kv 和各业务表都声明了 ON DELETE CASCADE；删除 workspace 时只删主表，
	// 让 SQLite 在同一事务内级联清理并生成响应快照，避免“实际已删但接口报错”。
	if _, err := tx.Exec(`DELETE FROM workspaces WHERE name = ?`, name); err != nil {
		return WorkspaceResponse{}, err
	}
	if _, err := tx.Exec(`UPDATE attachment_blobs SET ref_count = (SELECT COUNT(*) FROM attachments WHERE attachments.sha256 = attachment_blobs.sha256)`); err != nil {
		return WorkspaceResponse{}, err
	}
	result, err := listWorkspacesWith(tx, defaultWorkspaceID)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkspaceResponse{}, err
	}
	return result, nil
}

func workspaceRunningWorkWith(reader sqlQueryer, name string) (int, int, error) {
	var chatJobs, scheduledTasks int
	err := reader.QueryRow(`SELECT
  (SELECT COUNT(*) FROM chat_jobs WHERE workspace_id = ? AND status = 'running'),
  (SELECT COUNT(*) FROM scheduled_tasks WHERE workspace_id = ? AND running = 1)`, name, name).Scan(&chatJobs, &scheduledTasks)
	return chatJobs, scheduledTasks, err
}

func listWorkspaceSummariesWith(reader sqlQueryer, active string) ([]model.WorkspaceSummary, error) {
	type workspaceRow struct {
		name       string
		createdRaw string
		updatedRaw string
	}
	rows, err := reader.Query(`SELECT name, created_at, updated_at FROM workspaces`)
	if err != nil {
		return nil, err
	}
	workspaceRows := []workspaceRow{}
	for rows.Next() {
		var row workspaceRow
		if err := rows.Scan(&row.name, &row.createdRaw, &row.updatedRaw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		workspaceRows = append(workspaceRows, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	items := []model.WorkspaceSummary{}
	for _, row := range workspaceRows {
		item, err := workspaceSummaryFromDB(reader, row.name, active, row.createdRaw, row.updatedRaw)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == defaultWorkspaceID {
			return true
		}
		if items[j].Name == defaultWorkspaceID {
			return false
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	markActiveWorkspace(items, active)
	return items, nil
}

func markActiveWorkspace(items []model.WorkspaceSummary, requested string) {
	requested = strings.TrimSpace(requested)
	resolved := ""
	for _, item := range items {
		if item.Name == requested {
			resolved = requested
			break
		}
	}
	if resolved == "" {
		for _, item := range items {
			if item.Name == defaultWorkspaceID {
				resolved = defaultWorkspaceID
				break
			}
		}
	}
	if resolved == "" && len(items) > 0 {
		resolved = items[0].Name
	}
	for i := range items {
		items[i].Active = items[i].Name == resolved
	}
}

func activeWorkspaceFromSummaries(items []model.WorkspaceSummary) string {
	for _, item := range items {
		if item.Active {
			return item.Name
		}
	}
	return defaultWorkspaceID
}

func (s *Store) listWorkspaceIDsLocked() ([]string, error) {
	return listWorkspaceIDsWith(s.db)
}

func listWorkspaceIDsWith(reader sqlQueryer) ([]string, error) {
	rows, err := reader.Query(`SELECT name FROM workspaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func workspaceSummaryFromDB(reader sqlQueryer, name string, active string, createdRaw string, updatedRaw string) (model.WorkspaceSummary, error) {
	createdAt := parseDBTime(createdRaw)
	updatedAt := parseDBTime(updatedRaw)
	var count int
	var latest sql.NullString
	if err := reader.QueryRow(`SELECT COUNT(*), MAX(updated_at) FROM sessions WHERE workspace_id = ?`, name).Scan(&count, &latest); err != nil {
		return model.WorkspaceSummary{}, err
	}
	if latest.Valid {
		if t := parseDBTime(latest.String); t.After(updatedAt) {
			updatedAt = t
		}
	}
	return model.WorkspaceSummary{Name: name, Active: name == active, CreatedAt: createdAt, UpdatedAt: updatedAt, Count: count}, nil
}
