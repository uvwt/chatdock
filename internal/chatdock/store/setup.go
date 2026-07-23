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
	projectCount, err := projectCountWith(s.db)
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
	return SetupStatus{NeedsSetup: !hasProvider, HasModelProvider: hasProvider, HasAPIKey: hasKey, ProjectCount: projectCount, DataDir: dataDir}, nil
}

func (s *Store) InitializeSetup(input SetupInitRequest) (SetupStatus, error) {
	cfg := model.DefaultModelConfig()
	cfg.ProviderID = "provider_default"
	cfg.BaseURL = strings.TrimSpace(input.BaseURL)
	cfg.APIKey = strings.TrimSpace(input.APIKey)
	cfg.Model = strings.TrimSpace(input.Model)
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = model.DefaultModelConfig().SystemPrompt
	}
	cfg = model.NormalizeModelConfig(cfg)

	if err := s.initializeSetupRecords(cfg); err != nil {
		return SetupStatus{}, err
	}
	return s.SetupStatus()
}

func (s *Store) initializeSetupRecords(cfg model.ModelConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	if err := upsertProviderFromConfigWith(tx, cfg); err != nil {
		return err
	}
	if err := setGlobalJSONWith(tx, "config", cfg, now); err != nil {
		return err
	}
	if err := setGlobalRawWith(tx, "mcp", DefaultMCPConfig(), now); err != nil {
		return err
	}
	return tx.Commit()
}
