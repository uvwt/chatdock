package store

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) ModelConfig(workspaceID string) (model.ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelConfigForWorkspaceLocked(workspaceID)
}

func (s *Store) BestEffortModelConfig(workspaceID string) model.ModelConfig {
	cfg, err := s.ModelConfig(workspaceID)
	if err != nil {
		return model.DefaultModelConfig()
	}
	return cfg
}

func (s *Store) SaveModelConfig(workspaceID string, next model.ModelConfig) (model.ModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ModelConfig{}, err
	}
	return s.saveModelConfigForWorkspaceLocked(workspaceID, next)
}

func (s *Store) saveModelConfigForWorkspaceLocked(workspaceID string, next model.ModelConfig) (model.ModelConfig, error) {
	current, err := s.modelConfigForWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ModelConfig{}, err
	}
	requestedBaseURL := strings.TrimSpace(next.BaseURL)
	requestedProviderID := strings.TrimSpace(next.ProviderID)
	if requestedProviderID == "" {
		if requestedBaseURL != "" {
			next.ProviderID = providerIDFromWorkspace(workspaceID)
		} else {
			next.ProviderID = current.ProviderID
		}
	}
	if strings.TrimSpace(next.BaseURL) == "" {
		next.BaseURL = current.BaseURL
	}
	if strings.TrimSpace(next.APIKey) == "" || isMaskedSecret(next.APIKey) {
		next.APIKey = current.APIKey
	}
	if strings.TrimSpace(next.Model) == "" {
		next.Model = current.Model
	}
	if len(next.Models) == 0 {
		next.Models = append([]string(nil), current.Models...)
	}
	if strings.TrimSpace(next.EmbeddingAPIKey) == "" || isMaskedSecret(next.EmbeddingAPIKey) {
		next.EmbeddingAPIKey = current.EmbeddingAPIKey
	}
	embeddingChanged := strings.TrimSpace(next.EmbeddingBaseURL) != current.EmbeddingBaseURL || strings.TrimSpace(next.EmbeddingModel) != current.EmbeddingModel
	if strings.TrimSpace(next.SystemPrompt) == "" {
		next.SystemPrompt = current.SystemPrompt
	}
	next = model.NormalizeModelConfig(next)

	tx, err := s.db.Begin()
	if err != nil {
		return model.ModelConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if embeddingChanged {
		if err := deleteToolEmbeddingsWith(tx, workspaceID); err != nil {
			return model.ModelConfig{}, err
		}
	}
	if requestedBaseURL != "" {
		if err := upsertProviderFromConfigWith(tx, workspaceID, next); err != nil {
			return model.ModelConfig{}, err
		}
	}
	merged, err := applyProviderToConfigWith(tx, next)
	if err != nil {
		return model.ModelConfig{}, err
	}
	if err := setWorkspaceJSONWith(tx, workspaceID, "config", merged, time.Now()); err != nil {
		return model.ModelConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ModelConfig{}, err
	}
	return merged, nil
}

func upsertProviderFromConfigWith(db sqlQueryWriter, workspaceID string, cfg model.ModelConfig) error {
	records, err := loadModelProviderRecordsWith(db)
	if err != nil {
		return err
	}
	now := time.Now()
	id := normalizeProviderID(cfg.ProviderID)
	if id == "" {
		id = providerIDFromWorkspace(workspaceID)
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		records[i].BaseURL = strings.TrimSpace(cfg.BaseURL)
		records[i].APIKeys = upsertProviderAPIKey(records[i].APIKeys, records[i].SelectedKeyID, cfg.APIKey, now)
		records[i].DefaultModel = strings.TrimSpace(cfg.Model)
		records[i].Models = normalizeProviderModelNames(cfg.Models, cfg.Model)
		records[i].Enabled = strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != ""
		records[i].UpdatedAt = now
		records[i] = normalizeModelProviderRecord(records[i])
		return saveModelProviderRecordsWith(db, records)
	}
	record := normalizeModelProviderRecord(modelProviderRecord{ID: id, Name: providerDisplayName(workspaceID, cfg), Type: "openai-compatible", BaseURL: strings.TrimSpace(cfg.BaseURL), APIKeys: upsertProviderAPIKey(nil, "", cfg.APIKey, now), DefaultModel: strings.TrimSpace(cfg.Model), Models: normalizeProviderModelNames(cfg.Models, cfg.Model), TimeoutMS: 120000, Enabled: strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "", CreatedAt: now, UpdatedAt: now})
	records = append(records, record)
	return saveModelProviderRecordsWith(db, records)
}
