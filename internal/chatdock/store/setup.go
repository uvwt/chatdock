package store

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) SetupStatus() (SetupStatus, error) {
	s.mu.RLock()
	dataDir := s.dataDir
	providers, providerErr := s.loadModelProviderRecordsLocked()
	s.mu.RUnlock()
	if providerErr != nil {
		return SetupStatus{}, providerErr
	}
	workspaces, err := s.listWorkspaceSummaries(defaultWorkspaceID)
	if err != nil {
		return SetupStatus{}, err
	}
	hasProvider := false
	hasKey := false
	for _, provider := range providers {
		if strings.TrimSpace(provider.BaseURL) != "" && strings.TrimSpace(provider.DefaultModel) != "" && provider.Enabled {
			hasProvider = true
		}
		if strings.TrimSpace(provider.APIKey) != "" {
			hasKey = true
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
	s.mu.Lock()
	if exists, err := s.workspaceExistsLocked(name); err != nil {
		s.mu.Unlock()
		return SetupStatus{}, err
	} else if !exists {
		if err := s.insertWorkspaceLocked(name, time.Now()); err != nil {
			s.mu.Unlock()
			return SetupStatus{}, err
		}
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
	if err := s.upsertProviderFromConfigLocked(name, cfg); err != nil {
		s.mu.Unlock()
		return SetupStatus{}, err
	}
	err = s.setWorkspaceJSONLocked(name, "config", cfg)
	if err == nil {
		err = s.setWorkspaceRawLocked(name, "mcp", DefaultMCPConfig())
	}
	s.mu.Unlock()
	if err != nil {
		return SetupStatus{}, err
	}
	return s.SetupStatus()
}
