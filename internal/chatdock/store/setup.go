package store

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) SetupStatus() (SetupStatus, error) {
	s.mu.RLock()
	dataDir := s.dataDir
	providers, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		s.mu.RUnlock()
		return SetupStatus{}, err
	}
	workspaces, err := listWorkspaceSummariesWith(s.db, defaultWorkspaceID)
	s.mu.RUnlock()
	if err != nil {
		return SetupStatus{}, err
	}
	hasProvider := false
	hasKey := false
	for _, provider := range providers {
		if strings.TrimSpace(provider.BaseURL) != "" && strings.TrimSpace(provider.DefaultModel) != "" && provider.Enabled {
			hasProvider = true
		}
		for _, key := range provider.APIKeys {
			if strings.TrimSpace(key.APIKey) != "" && key.Enabled {
				hasKey = true
			}
		}
	}
	return SetupStatus{NeedsSetup: len(workspaces) == 0 || !hasProvider, HasModelProvider: hasProvider, HasAPIKey: hasKey, HasWorkspace: len(workspaces) > 0, ActiveWorkspace: defaultWorkspaceID, WorkspaceCount: len(workspaces), DataDir: dataDir}, nil
}

func (s *Store) InitializeSetup(input SetupInitRequest) (SetupStatus, error) {
	name := strings.TrimSpace(input.WorkspaceName)
	if name == "" {
		name = defaultWorkspaceID
	}
	name, err := normalizeWorkspaceID(name)
	if err != nil {
		return SetupStatus{}, err
	}
	cfg := model.DefaultModelConfig()
	cfg.ProviderID = providerIDFromWorkspace(name)
	cfg.BaseURL = strings.TrimSpace(input.BaseURL)
	cfg.APIKey = strings.TrimSpace(input.APIKey)
	cfg.Model = strings.TrimSpace(input.Model)
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = model.DefaultModelConfig().SystemPrompt
	}
	cfg = model.NormalizeModelConfig(cfg)

	if err := s.initializeSetupRecords(name, cfg); err != nil {
		return SetupStatus{}, err
	}
	return s.SetupStatus()
}

func (s *Store) initializeSetupRecords(name string, cfg model.ModelConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO workspaces(name, created_at, updated_at) VALUES(?, ?, ?)`, name, formatDBTime(now), formatDBTime(now)); err != nil {
		return err
	}
	if err := upsertProviderFromConfigWith(tx, name, cfg); err != nil {
		return err
	}
	if err := setWorkspaceJSONWith(tx, name, "config", cfg, now); err != nil {
		return err
	}
	if err := setWorkspaceRawWith(tx, name, "mcp", DefaultMCPConfig(), now); err != nil {
		return err
	}
	return tx.Commit()
}
