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

// 工作空间、模型配置和 MCP 配置是一组同生命周期的 model.WorkspaceSummary 数据。
// 这些方法只负责选择、校验和保存当前工作空间配置，不直接处理消息追加。

func (s *Store) ActivePrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activePrompt
}

func (s *Store) ListPrompts() (model.WorkspaceListResponse, error) {
	s.mu.RLock()
	active := s.activePrompt
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: active, Prompts: prompts}, nil
}

func (s *Store) CreatePrompt(input model.CreateWorkspaceRequest) (model.WorkspaceListResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if exists {
		return model.WorkspaceListResponse{}, fmt.Errorf("prompt already exists: %s", name)
	}

	now := time.Now()
	if err := s.insertPromptLocked(name, now); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	cfg := s.modelCfg
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg = model.DefaultModelConfig()
	}
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = model.DefaultModelConfig().SystemPrompt
	}
	cfg = model.NormalizeModelConfig(cfg)
	if err := s.setPromptJSONLocked(name, "config", cfg); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if err := s.setPromptRawLocked(name, "mcp", DefaultMCPConfig()); err != nil {
		return model.WorkspaceListResponse{}, err
	}

	if err := s.loadPromptLocked(name); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) DeletePrompt(input model.WorkspaceIDRequest) (model.WorkspaceListResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if name == defaultPromptName {
		return model.WorkspaceListResponse{}, fmt.Errorf("default workspace cannot be deleted")
	}
	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if !exists {
		return model.WorkspaceListResponse{}, fmt.Errorf("prompt not found: %s", name)
	}
	names, err := s.listPromptNamesLocked()
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if len(names) <= 1 {
		return model.WorkspaceListResponse{}, fmt.Errorf("last workspace cannot be deleted")
	}
	if name == s.activePrompt {
		fallback := defaultPromptName
		if fallback == name {
			fallback = ""
		}
		for _, candidate := range names {
			if candidate != name && (fallback == "" || candidate == defaultPromptName) {
				fallback = candidate
			}
		}
		if fallback == "" {
			return model.WorkspaceListResponse{}, fmt.Errorf("no fallback workspace available")
		}
		if err := s.loadPromptLocked(fallback); err != nil {
			return model.WorkspaceListResponse{}, err
		}
	}
	// prompt_kv 和 sessions 都声明了 ON DELETE CASCADE；删除 workspace 时必须只删 prompts 主表，
	// 让 SQLite 在同一个连接内级联清理关联数据，避免前后端各自补删造成状态不一致。
	if _, err := s.db.Exec(`DELETE FROM prompts WHERE name = ?`, name); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) SelectPrompt(input model.WorkspaceIDRequest) (model.WorkspaceListResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	if !exists {
		return model.WorkspaceListResponse{}, fmt.Errorf("prompt not found: %s", name)
	}
	if err := s.loadPromptLocked(name); err != nil {
		return model.WorkspaceListResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return model.WorkspaceListResponse{}, err
	}
	return model.WorkspaceListResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) loadPromptLocked(name string) error {
	name, err := normalizePromptName(name)
	if err != nil {
		return err
	}
	if err := s.ensurePromptLocked(name); err != nil {
		return err
	}
	s.activePrompt = name
	s.sessions = make(map[string]*model.Session)
	if err := s.loadModelConfigLocked(); err != nil {
		return err
	}
	return s.loadSessionsLocked()
}

func (s *Store) loadModelConfigLocked() error {
	raw, ok, err := s.getPromptRawLocked(s.activePrompt, "config")
	if err != nil {
		return err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		s.modelCfg = model.DefaultModelConfig()
		return s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
	}
	if err := json.Unmarshal([]byte(raw), &s.modelCfg); err != nil {
		return err
	}
	s.modelCfg = model.NormalizeModelConfig(s.modelCfg)
	merged, err := s.applyProviderToConfigLocked(s.modelCfg)
	if err != nil {
		return err
	}
	s.modelCfg = merged
	return nil
}

func (s *Store) listPrompts(active string) ([]model.WorkspaceSummary, error) {
	type promptRow struct {
		name       string
		createdRaw string
		updatedRaw string
	}
	rows, err := s.db.Query(`SELECT name, created_at, updated_at FROM prompts`)
	if err != nil {
		return nil, err
	}
	promptRows := []promptRow{}
	for rows.Next() {
		var row promptRow
		if err := rows.Scan(&row.name, &row.createdRaw, &row.updatedRaw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		promptRows = append(promptRows, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	items := []model.WorkspaceSummary{}
	for _, row := range promptRows {
		item, err := s.promptSummaryFromDB(row.name, active, row.createdRaw, row.updatedRaw)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == defaultPromptName {
			return true
		}
		if items[j].Name == defaultPromptName {
			return false
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *Store) listPromptNamesLocked() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM prompts`)
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

func (s *Store) listPromptsLocked() ([]model.WorkspaceSummary, error) {
	return s.listPrompts(s.activePrompt)
}

func (s *Store) promptSummaryFromDB(name string, active string, createdRaw string, updatedRaw string) (model.WorkspaceSummary, error) {
	createdAt := parseDBTime(createdRaw)
	updatedAt := parseDBTime(updatedRaw)
	var count int
	var latest sql.NullString
	if err := s.db.QueryRow(`SELECT COUNT(*), MAX(updated_at) FROM sessions WHERE prompt = ?`, name).Scan(&count, &latest); err != nil {
		return model.WorkspaceSummary{}, err
	}
	if latest.Valid {
		if t := parseDBTime(latest.String); t.After(updatedAt) {
			updatedAt = t
		}
	}
	return model.WorkspaceSummary{Name: name, Active: name == active, CreatedAt: createdAt, UpdatedAt: updatedAt, Count: count}, nil
}
