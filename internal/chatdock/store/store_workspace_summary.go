package store

import (
	"database/sql"
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
	if err := s.ensureWorkspaceLocked(name); err != nil {
		return err
	}
	if raw, ok, err := s.getWorkspaceRawLocked(name, "config"); err != nil {
		return err
	} else if !ok || strings.TrimSpace(raw) == "" {
		if err := s.setWorkspaceJSONLocked(name, "config", model.DefaultModelConfig()); err != nil {
			return err
		}
	}
	if raw, ok, err := s.getWorkspaceRawLocked(name, "mcp"); err != nil {
		return err
	} else if !ok || strings.TrimSpace(raw) == "" {
		if err := s.setWorkspaceRawLocked(name, "mcp", DefaultMCPConfig()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListWorkspaceSummaries(active string) (model.WorkspaceListResponse, error) {
	active = strings.TrimSpace(active)
	if active == "" {
		active = defaultWorkspaceID
	}
	workspaces, err := s.listWorkspaceSummaries(active)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: active, Workspaces: workspaces}, nil
}

func (s *Store) CreateWorkspace(input model.CreateWorkspaceRequest) (model.WorkspaceListResponse, error) {
	name, err := normalizeWorkspaceID(input.Name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.workspaceExistsLocked(name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if exists {
		return model.WorkspaceListResponse{}, fmt.Errorf("workspace already exists: %s", name)
	}

	now := time.Now()
	if err := s.insertWorkspaceLocked(name, now); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	cfg, err := s.modelConfigForWorkspaceLocked(defaultWorkspaceID)
	if err != nil {
		cfg = model.DefaultModelConfig()
	}
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = model.DefaultModelConfig().SystemPrompt
	}
	cfg = model.NormalizeModelConfig(cfg)
	if err := s.setWorkspaceJSONLocked(name, "config", cfg); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if err := s.setWorkspaceRawLocked(name, "mcp", DefaultMCPConfig()); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	workspaces, err := s.listWorkspaceSummaries(name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: name, Workspaces: workspaces}, nil
}

func (s *Store) DeleteWorkspace(input model.WorkspaceIDRequest) (model.WorkspaceListResponse, error) {
	name, err := normalizeWorkspaceID(input.Name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if name == defaultWorkspaceID {
		return model.WorkspaceListResponse{}, fmt.Errorf("default workspace cannot be deleted")
	}
	exists, err := s.workspaceExistsLocked(name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if !exists {
		return model.WorkspaceListResponse{}, fmt.Errorf("workspace not found: %s", name)
	}
	names, err := s.listWorkspaceIDsLocked()
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if len(names) <= 1 {
		return model.WorkspaceListResponse{}, fmt.Errorf("last workspace cannot be deleted")
	}
	// workspace_kv 和各业务表都声明了 ON DELETE CASCADE；删除 workspace 时只删主表，
	// 让 SQLite 在同一连接内级联清理，避免前后端各自补删造成状态不一致。
	if _, err := s.db.Exec(`DELETE FROM workspaces WHERE name = ?`, name); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	active := defaultWorkspaceID
	workspaces, err := s.listWorkspaceSummaries(active)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: active, Workspaces: workspaces}, nil
}

func (s *Store) listWorkspaceSummaries(active string) ([]model.WorkspaceSummary, error) {
	type workspaceRow struct {
		name       string
		createdRaw string
		updatedRaw string
	}
	rows, err := s.db.Query(`SELECT name, created_at, updated_at FROM workspaces`)
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
		item, err := s.workspaceSummaryFromDB(row.name, active, row.createdRaw, row.updatedRaw)
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
	return items, nil
}

func (s *Store) listWorkspaceIDsLocked() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM workspaces`)
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

func (s *Store) workspaceSummaryFromDB(name string, active string, createdRaw string, updatedRaw string) (model.WorkspaceSummary, error) {
	createdAt := parseDBTime(createdRaw)
	updatedAt := parseDBTime(updatedRaw)
	var count int
	var latest sql.NullString
	if err := s.db.QueryRow(`SELECT COUNT(*), MAX(updated_at) FROM sessions WHERE workspace_id = ?`, name).Scan(&count, &latest); err != nil {
		return model.WorkspaceSummary{}, err
	}
	if latest.Valid {
		if t := parseDBTime(latest.String); t.After(updatedAt) {
			updatedAt = t
		}
	}
	return model.WorkspaceSummary{Name: name, Active: name == active, CreatedAt: createdAt, UpdatedAt: updatedAt, Count: count}, nil
}
