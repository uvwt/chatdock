package chatdock

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// 工作空间、模型配置和 MCP 配置是一组同生命周期的 PromptSpace 数据。
// 这些方法只负责选择、校验和保存当前工作空间配置，不直接处理消息追加。

func (s *Store) ActivePrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activePrompt
}

func (s *Store) ListPrompts() (PromptResponse, error) {
	s.mu.RLock()
	active := s.activePrompt
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: active, Prompts: prompts}, nil
}

func (s *Store) CreatePrompt(input CreatePromptRequest) (PromptResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return PromptResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return PromptResponse{}, err
	}
	if exists {
		return PromptResponse{}, fmt.Errorf("prompt already exists: %s", name)
	}

	now := time.Now()
	if err := s.insertPromptLocked(name, now); err != nil {
		return PromptResponse{}, err
	}
	cfg := s.modelCfg
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg = DefaultModelConfig()
	}
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = DefaultModelConfig().SystemPrompt
	}
	cfg = NormalizeModelConfig(cfg)
	if err := s.setPromptJSONLocked(name, "config", cfg); err != nil {
		return PromptResponse{}, err
	}
	if err := s.setPromptRawLocked(name, "mcp", DefaultMCPConfig()); err != nil {
		return PromptResponse{}, err
	}

	if err := s.loadPromptLocked(name); err != nil {
		return PromptResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) DeletePrompt(input SelectPromptRequest) (PromptResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return PromptResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if name == defaultPromptName {
		return PromptResponse{}, fmt.Errorf("default workspace cannot be deleted")
	}
	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return PromptResponse{}, err
	}
	if !exists {
		return PromptResponse{}, fmt.Errorf("prompt not found: %s", name)
	}
	names, err := s.listPromptNamesLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	if len(names) <= 1 {
		return PromptResponse{}, fmt.Errorf("last workspace cannot be deleted")
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
			return PromptResponse{}, fmt.Errorf("no fallback workspace available")
		}
		if err := s.loadPromptLocked(fallback); err != nil {
			return PromptResponse{}, err
		}
	}
	// prompt_kv 和 sessions 都声明了 ON DELETE CASCADE；删除 workspace 时必须只删 prompts 主表，
	// 让 SQLite 在同一个连接内级联清理关联数据，避免前后端各自补删造成状态不一致。
	if _, err := s.db.Exec(`DELETE FROM prompts WHERE name = ?`, name); err != nil {
		return PromptResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) SelectPrompt(input SelectPromptRequest) (PromptResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return PromptResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return PromptResponse{}, err
	}
	if !exists {
		return PromptResponse{}, fmt.Errorf("prompt not found: %s", name)
	}
	if err := s.loadPromptLocked(name); err != nil {
		return PromptResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) GetMCPConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, ok, err := s.getPromptRawLocked(s.activePrompt, "mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		return content, s.setPromptRawLocked(s.activePrompt, "mcp", content)
	}
	return content, nil
}

func (s *Store) GetEffectiveMCPConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, ok, err := s.getPromptRawLocked(s.activePrompt, "mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		if err := s.setPromptRawLocked(s.activePrompt, "mcp", content); err != nil {
			return "", err
		}
	}
	if mcpConfigHasServers(content) || s.activePrompt == defaultPromptName {
		return content, nil
	}

	fallback, ok, err := s.getPromptRawLocked(defaultPromptName, "mcp")
	if err != nil {
		return "", err
	}
	if ok && mcpConfigHasServers(fallback) {
		return fallback, nil
	}
	return content, nil
}

func mcpConfigHasServers(content string) bool {
	cfg, err := ParseMCPConfig(content)
	return err == nil && len(cfg.Servers) > 0
}

func (s *Store) SaveMCPConfig(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = DefaultMCPConfig()
	}
	var probe any
	if err := json.Unmarshal([]byte(content), &probe); err != nil {
		return "", fmt.Errorf("mcp config must be valid json: %w", err)
	}
	pretty, err := prettyJSON(content)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return pretty, s.setPromptRawLocked(s.activePrompt, "mcp", pretty)
}

func DefaultMCPConfig() string {
	return `{
  "servers": {}
}
`
}

func (s *Store) GetModelConfig() ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelCfg
}

func (s *Store) SaveModelConfig(next ModelConfig) (ModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
		next.APIKey = s.modelCfg.APIKey
	}

	s.modelCfg = NormalizeModelConfig(next)
	return s.modelCfg, s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
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
	s.sessions = make(map[string]*Session)
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
		s.modelCfg = DefaultModelConfig()
		return s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
	}
	if err := json.Unmarshal([]byte(raw), &s.modelCfg); err != nil {
		return err
	}
	s.modelCfg = NormalizeModelConfig(s.modelCfg)
	return nil
}

func (s *Store) listPrompts(active string) ([]PromptSpace, error) {
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

	items := []PromptSpace{}
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

func (s *Store) listPromptsLocked() ([]PromptSpace, error) {
	return s.listPrompts(s.activePrompt)
}

func (s *Store) promptSummaryFromDB(name string, active string, createdRaw string, updatedRaw string) (PromptSpace, error) {
	createdAt := parseDBTime(createdRaw)
	updatedAt := parseDBTime(updatedRaw)
	var count int
	var latest sql.NullString
	if err := s.db.QueryRow(`SELECT COUNT(*), MAX(updated_at) FROM sessions WHERE prompt = ?`, name).Scan(&count, &latest); err != nil {
		return PromptSpace{}, err
	}
	if latest.Valid {
		if t := parseDBTime(latest.String); t.After(updatedAt) {
			updatedAt = t
		}
	}
	return PromptSpace{Name: name, Active: name == active, CreatedAt: createdAt, UpdatedAt: updatedAt, Count: count}, nil
}

func (s *Store) promptExistsLocked(name string) (bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT name FROM prompts WHERE name = ?`, name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ensurePromptLocked(name string) error {
	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.insertPromptLocked(name, time.Now())
}

func (s *Store) insertPromptLocked(name string, now time.Time) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO prompts(name, created_at, updated_at) VALUES(?, ?, ?)`, name, formatDBTime(now), formatDBTime(now))
	return err
}

func (s *Store) touchPromptLocked(name string, now time.Time) error {
	_, err := s.db.Exec(`UPDATE prompts SET updated_at = ? WHERE name = ?`, formatDBTime(now), name)
	return err
}

func (s *Store) getPromptRawLocked(prompt string, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM prompt_kv WHERE prompt = ? AND key = ?`, prompt, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) setPromptRawLocked(prompt string, key string, value string) error {
	if err := s.ensurePromptLocked(prompt); err != nil {
		return err
	}
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO prompt_kv(prompt, key, value, updated_at) VALUES(?, ?, ?, ?)
ON CONFLICT(prompt, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, prompt, key, value, formatDBTime(now))
	if err != nil {
		return err
	}
	return s.touchPromptLocked(prompt, now)
}

func (s *Store) setPromptJSONLocked(prompt string, key string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return s.setPromptRawLocked(prompt, key, string(raw)+"\n")
}

func normalizePromptName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("prompt name is empty")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("prompt name is invalid")
	}
	if name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("prompt name cannot contain path separators")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("prompt name contains control characters")
		}
	}
	return name, nil
}
