package chatdock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SetupStatus struct {
	NeedsSetup       bool   `json:"needs_setup"`
	HasModelProvider bool   `json:"has_model_provider"`
	HasAPIKey        bool   `json:"has_api_key"`
	HasWorkspace     bool   `json:"has_workspace"`
	ActiveWorkspace  string `json:"active_workspace"`
	WorkspaceCount   int    `json:"workspace_count"`
	DataDir          string `json:"data_dir"`
}

type SetupInitRequest struct {
	WorkspaceName string `json:"workspace_name"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	Model         string `json:"model"`
	SystemPrompt  string `json:"system_prompt"`
}

type ModelProvider struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	BaseURL       string    `json:"base_url"`
	HasAPIKey     bool      `json:"has_api_key"`
	APIKeyMasked  string    `json:"api_key_masked,omitempty"`
	DefaultModel  string    `json:"default_model"`
	TimeoutMS     int       `json:"timeout_ms"`
	Enabled       bool      `json:"enabled"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	WorkspaceName string    `json:"workspace_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Workspace struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description,omitempty"`
	Icon              string    `json:"icon,omitempty"`
	ProviderID        string    `json:"provider_id"`
	Model             string    `json:"model"`
	SystemPrompt      string    `json:"system_prompt"`
	ContextLimit      int       `json:"context_limit"`
	Temperature       float64   `json:"temperature"`
	HideThinking      bool      `json:"hide_thinking"`
	EnableReasoning   bool      `json:"enable_reasoning"`
	SkillCount        int       `json:"skill_count"`
	EnabledSkillCount int       `json:"enabled_skill_count"`
	TaskCount         int       `json:"task_count"`
	SessionCount      int       `json:"session_count"`
	Active            bool      `json:"active"`
	Archived          bool      `json:"archived"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type WorkspaceResponse struct {
	Active     string      `json:"active"`
	Workspaces []Workspace `json:"workspaces"`
}

type PromptPreviewResponse struct {
	WorkspaceID   string   `json:"workspace_id"`
	WorkspaceName string   `json:"workspace_name"`
	SkillNames    []string `json:"skill_names"`
	Content       string   `json:"content"`
}

type BackupInfo struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"size_bytes"`
	UpdatedAt  time.Time `json:"updated_at"`
	AgeSeconds int64     `json:"age_seconds"`
}

type DataStatus struct {
	DataDir                string       `json:"data_dir"`
	DatabasePath           string       `json:"database_path"`
	DatabaseExists         bool         `json:"database_exists"`
	DatabaseHealthy        bool         `json:"database_healthy"`
	DatabaseCheck          string       `json:"database_check,omitempty"`
	DatabaseWarning        string       `json:"database_warning,omitempty"`
	DatabaseSizeBytes      int64        `json:"database_size_bytes"`
	WALEnabled             bool         `json:"wal_enabled"`
	SHMExists              bool         `json:"shm_exists"`
	BackupDir              string       `json:"backup_dir,omitempty"`
	BackupCheckedDirs      []string     `json:"backup_checked_dirs,omitempty"`
	BackupCount            int          `json:"backup_count"`
	BackupHealthy          bool         `json:"backup_healthy"`
	BackupWarning          string       `json:"backup_warning,omitempty"`
	LatestBackupPath       string       `json:"latest_backup_path,omitempty"`
	LatestBackupSizeBytes  int64        `json:"latest_backup_size_bytes,omitempty"`
	LatestBackupAt         time.Time    `json:"latest_backup_at,omitempty"`
	LatestBackupAgeSeconds int64        `json:"latest_backup_age_seconds,omitempty"`
	Backups                []BackupInfo `json:"backups,omitempty"`
	ActiveWorkspace        string       `json:"active_workspace"`
	WorkspaceCount         int          `json:"workspace_count"`
	SessionCount           int          `json:"session_count"`
}

type MCPServerStatus struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Disabled     bool   `json:"disabled"`
	AuthType     string `json:"auth_type,omitempty"`
	HasToken     bool   `json:"has_token"`
	AllowCount   int    `json:"allow_count"`
	DenyCount    int    `json:"deny_count"`
	ConfirmCount int    `json:"confirm_count"`
	TimeoutMS    int    `json:"timeout_ms"`
	CacheTTLMS   int    `json:"cache_ttl_ms"`
	LastStatus   string `json:"last_status"`
	LastError    string `json:"last_error,omitempty"`
}

type MCPStatusResponse struct {
	Servers []MCPServerStatus `json:"servers"`
}

func (s *Store) SetupStatus() (SetupStatus, error) {
	s.mu.RLock()
	active := s.activePrompt
	cfg := s.modelCfg
	dataDir := s.dataDir
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return SetupStatus{}, err
	}
	defaultCfg := DefaultModelConfig()
	hasProvider := strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != ""
	hasKey := strings.TrimSpace(cfg.APIKey) != ""
	looksDefaultWithoutKey := !hasKey && cfg.BaseURL == defaultCfg.BaseURL && cfg.Model == defaultCfg.Model
	return SetupStatus{
		NeedsSetup:       !hasProvider || len(prompts) == 0 || looksDefaultWithoutKey,
		HasModelProvider: hasProvider,
		HasAPIKey:        hasKey,
		HasWorkspace:     len(prompts) > 0,
		ActiveWorkspace:  active,
		WorkspaceCount:   len(prompts),
		DataDir:          dataDir,
	}, nil
}

func (s *Store) InitializeSetup(input SetupInitRequest) (SetupStatus, error) {
	name := strings.TrimSpace(input.WorkspaceName)
	if name == "" {
		name = defaultPromptName
	}
	name, err := normalizePromptName(name)
	if err != nil {
		return SetupStatus{}, err
	}

	s.mu.Lock()
	if exists, err := s.promptExistsLocked(name); err != nil {
		s.mu.Unlock()
		return SetupStatus{}, err
	} else if !exists {
		if err := s.insertPromptLocked(name, time.Now()); err != nil {
			s.mu.Unlock()
			return SetupStatus{}, err
		}
	}
	if err := s.loadPromptLocked(name); err != nil {
		s.mu.Unlock()
		return SetupStatus{}, err
	}
	cfg := s.modelCfg
	cfg.BaseURL = strings.TrimSpace(input.BaseURL)
	cfg.APIKey = strings.TrimSpace(input.APIKey)
	cfg.Model = strings.TrimSpace(input.Model)
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = DefaultModelConfig().SystemPrompt
	}
	s.modelCfg = NormalizeModelConfig(cfg)
	err = s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
	s.mu.Unlock()
	if err != nil {
		return SetupStatus{}, err
	}
	return s.SetupStatus()
}

func (s *Store) ListModelProviders() ([]ModelProvider, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	providers := make([]ModelProvider, 0, len(workspaces.Workspaces))
	for _, ws := range workspaces.Workspaces {
		cfg, err := s.modelConfigForPrompt(ws.ID)
		if err != nil {
			return nil, err
		}
		providers = append(providers, ModelProvider{
			ID:            cfg.ProviderID,
			Name:          providerName(ws.Name, cfg),
			Type:          "openai-compatible",
			BaseURL:       cfg.BaseURL,
			HasAPIKey:     strings.TrimSpace(cfg.APIKey) != "",
			APIKeyMasked:  maskSecret(cfg.APIKey),
			DefaultModel:  cfg.Model,
			TimeoutMS:     120000,
			Enabled:       strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "",
			WorkspaceID:   ws.ID,
			WorkspaceName: ws.Name,
			CreatedAt:     ws.CreatedAt,
			UpdatedAt:     ws.UpdatedAt,
		})
	}
	return providers, nil
}

func (s *Store) ListWorkspaces() (WorkspaceResponse, error) {
	s.mu.RLock()
	active := s.activePrompt
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	items := make([]Workspace, 0, len(prompts))
	for _, prompt := range prompts {
		cfg, err := s.modelConfigForPrompt(prompt.Name)
		if err != nil {
			return WorkspaceResponse{}, err
		}
		skills, _ := s.skillsForPrompt(prompt.Name)
		tasks, _ := s.scheduledTasksForPrompt(prompt.Name)
		enabledSkills := 0
		for _, skill := range skills {
			if skill.Enabled {
				enabledSkills++
			}
		}
		items = append(items, Workspace{
			ID:                prompt.Name,
			Name:              prompt.Name,
			Description:       workspaceDescription(prompt.Name),
			Icon:              "message-circle",
			ProviderID:        cfg.ProviderID,
			Model:             cfg.Model,
			SystemPrompt:      cfg.SystemPrompt,
			ContextLimit:      cfg.MaxContextMessages,
			Temperature:       cfg.Temperature,
			HideThinking:      cfg.HideThinking,
			EnableReasoning:   cfg.EnableThinking,
			SkillCount:        len(skills),
			EnabledSkillCount: enabledSkills,
			TaskCount:         len(tasks),
			SessionCount:      prompt.Count,
			Active:            prompt.Active,
			Archived:          false,
			CreatedAt:         prompt.CreatedAt,
			UpdatedAt:         prompt.UpdatedAt,
		})
	}
	return WorkspaceResponse{Active: active, Workspaces: items}, nil
}

func (s *Store) WorkspaceConfig(workspaceID string) (PublicModelConfig, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if exists, err := s.promptExistsLocked(workspaceID); err != nil {
		return PublicModelConfig{}, err
	} else if !exists {
		return PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	cfg, err := s.modelConfigForPromptLocked(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	return ToPublicModelConfig(cfg), nil
}

func (s *Store) SaveWorkspaceConfig(workspaceID string, next ModelConfig) (PublicModelConfig, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	if !exists {
		return PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	current, err := s.modelConfigForPromptLocked(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
		next.APIKey = current.APIKey
	}
	if strings.TrimSpace(next.SystemPrompt) == "" {
		next.SystemPrompt = current.SystemPrompt
	}
	next = NormalizeModelConfig(next)
	// 工作空间配置可以在不切换当前会话空间的情况下保存；如果保存的是当前空间，同步内存态，避免后续聊天继续用旧配置。
	if workspaceID == s.activePrompt {
		s.modelCfg = next
	}
	if err := s.setPromptJSONLocked(workspaceID, "config", next); err != nil {
		return PublicModelConfig{}, err
	}
	return ToPublicModelConfig(next), nil
}

func (s *Store) PromptPreview(workspaceID string) (PromptPreviewResponse, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.activePrompt
	if err := s.loadPromptLocked(workspaceID); err != nil {
		return PromptPreviewResponse{}, err
	}
	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	cfg.Skills = skills
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	content := buildSystemPrompt(cfg)
	if previous != workspaceID {
		if restoreErr := s.loadPromptLocked(previous); restoreErr != nil && err == nil {
			err = restoreErr
		}
	}
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	return PromptPreviewResponse{WorkspaceID: workspaceID, WorkspaceName: workspaceID, SkillNames: names, Content: content}, nil
}

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

func (s *Store) modelConfigForPrompt(prompt string) (ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelConfigForPromptLocked(prompt)
}

func (s *Store) modelConfigForPromptLocked(prompt string) (ModelConfig, error) {
	raw, ok, err := s.getPromptRawLocked(prompt, "config")
	if err != nil {
		return ModelConfig{}, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return DefaultModelConfig(), nil
	}
	var cfg ModelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return ModelConfig{}, err
	}
	return NormalizeModelConfig(cfg), nil
}

func (s *Store) skillsForPrompt(prompt string) ([]Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok, err := s.getPromptRawLocked(prompt, "skills")
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return []Skill{}, err
	}
	var skills []Skill
	if err := json.Unmarshal([]byte(raw), &skills); err != nil {
		return nil, err
	}
	sortSkills(skills)
	return cloneSkills(skills), nil
}

func (s *Store) scheduledTasksForPrompt(prompt string) ([]ScheduledTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok, err := s.getPromptRawLocked(prompt, "scheduled_tasks")
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return []ScheduledTask{}, err
	}
	var tasks []ScheduledTask
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		return nil, err
	}
	sortScheduledTasks(tasks)
	return cloneScheduledTasks(tasks), nil
}

func providerName(workspace string, cfg ModelConfig) string {
	base := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cfg.BaseURL), "https://"), "http://")
	if base == "" {
		base = "OpenAI Compatible"
	}
	return workspace + " · " + base
}

func workspaceDescription(name string) string {
	if name == defaultPromptName {
		return "默认通用 AI 工作空间"
	}
	return "独立模型、提示词、技能、工具和自动化配置"
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "******"
	}
	return value[:4] + "******" + value[len(value)-4:]
}

func (a *App) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.store.SetupStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}

func (a *App) handleSetupInit(w http.ResponseWriter, r *http.Request) {
	var input SetupInitRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, err := a.store.InitializeSetup(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}

func (a *App) handleListModelProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := a.store.ListModelProviders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"providers": providers})
}

func (a *App) handleTestModelProvider(w http.ResponseWriter, r *http.Request) {
	cfg := a.store.GetModelConfig()
	if r.Body != nil {
		defer r.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "{}" {
			next := cfg
			if err := json.Unmarshal(raw, &next); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
				next.APIKey = cfg.APIKey
			}
			cfg = NormalizeModelConfig(next)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.client.TestModelProvider(ctx, cfg); err != nil {
		writeJSONResponse(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "provider_id": cfg.ProviderID, "model": cfg.Model})
}

func (a *App) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var input CreatePromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.store.CreatePrompt(input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleWorkspaceRoute(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workspaces/"), "/")
	parts := strings.Split(requestPath, "/")
	if len(parts) != 1 && len(parts) != 2 {
		writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
		return
	}

	workspaceID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if _, err := a.store.DeletePrompt(SelectPromptRequest{Name: workspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.ListWorkspaces()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
		return
	}
	// 配置中心使用 /api/workspaces 作为产品化资源入口；旧 /api/prompts 继续兼容侧栏提示词空间。
	if parts[1] == "select" && r.Method == http.MethodPost {
		if _, err := a.store.SelectPrompt(SelectPromptRequest{Name: workspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.ListWorkspaces()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if parts[1] == "config" && r.Method == http.MethodGet {
		cfg, err := a.store.WorkspaceConfig(workspaceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, cfg)
		return
	}
	if parts[1] == "config" && r.Method == http.MethodPost {
		var input ModelConfig
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg, err := a.store.SaveWorkspaceConfig(workspaceID, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, cfg)
		return
	}
	if len(parts) == 2 && parts[1] == "prompt-preview" && r.Method == http.MethodGet {
		preview, err := a.store.PromptPreview(workspaceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, preview)
		return
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
}

func (a *App) handleDataStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.store.DataStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}

func (a *App) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	setup, err := a.store.SetupStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	data, err := a.store.DataStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{
		"ok":       true,
		"name":     "ChatDock",
		"time":     time.Now(),
		"setup":    setup,
		"data":     data,
		"web_dir":  a.cfg.WebDir,
		"addr":     a.cfg.Addr,
		"database": filepath.Base(data.DatabasePath),
	})
}

func (a *App) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]MCPServerStatus, 0, len(names))
	for _, name := range names {
		server := cfg.Servers[name]
		status := MCPServerStatus{
			Name:         name,
			URL:          server.URL,
			Disabled:     server.Disabled,
			AuthType:     server.Auth.Type,
			HasToken:     server.bearerToken() != "",
			AllowCount:   len(server.AllowTools),
			DenyCount:    len(server.DenyTools),
			ConfirmCount: len(server.ConfirmTools),
			TimeoutMS:    server.TimeoutMS,
			CacheTTLMS:   server.CacheTTLMS,
			LastStatus:   "unknown",
		}
		if server.Disabled || strings.TrimSpace(server.URL) == "" {
			status.LastStatus = "disabled"
		} else if tools, err := a.mcpClient.ListServerTools(r.Context(), cfg, name); err != nil {
			status.LastStatus = "error"
			status.LastError = err.Error()
		} else {
			status.LastStatus = fmt.Sprintf("ok · %d tools", len(tools))
		}
		servers = append(servers, status)
	}
	writeJSONResponse(w, http.StatusOK, MCPStatusResponse{Servers: servers})
}
