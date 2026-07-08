package store

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) GetModelConfig() model.ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelCfg
}

func (s *Store) SaveModelConfig(next model.ModelConfig) (model.ModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.saveModelConfigForWorkspaceLocked(s.workspaceCacheID, next)
	if err != nil {
		return model.ModelConfig{}, err
	}
	s.modelCfg = cfg
	return cfg, nil
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
	if strings.TrimSpace(next.EmbeddingBaseURL) != current.EmbeddingBaseURL || strings.TrimSpace(next.EmbeddingModel) != current.EmbeddingModel {
		if err := s.deleteToolEmbeddingsForWorkspaceLocked(workspaceID); err != nil {
			return model.ModelConfig{}, err
		}
	}
	if strings.TrimSpace(next.SystemPrompt) == "" {
		next.SystemPrompt = current.SystemPrompt
	}
	next = model.NormalizeModelConfig(next)
	if requestedBaseURL != "" {
		if err := s.upsertProviderFromConfigLocked(workspaceID, next); err != nil {
			return model.ModelConfig{}, err
		}
	}
	merged, err := s.applyProviderToConfigLocked(next)
	if err != nil {
		return model.ModelConfig{}, err
	}
	if err := s.setWorkspaceJSONLocked(workspaceID, "config", merged); err != nil {
		return model.ModelConfig{}, err
	}
	return merged, nil
}

func (s *Store) upsertProviderFromConfigLocked(workspaceID string, cfg model.ModelConfig) error {
	records, err := s.loadModelProviderRecordsLocked()
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
		records[i].APIKey = strings.TrimSpace(cfg.APIKey)
		records[i].APIKeys = upsertLegacyAPIKeyRecord(records[i].APIKeys, cfg.APIKey, now)
		records[i].DefaultModel = strings.TrimSpace(cfg.Model)
		records[i].Models = normalizeProviderModelNames(cfg.Models, cfg.Model)
		records[i].Enabled = strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != ""
		records[i].UpdatedAt = now
		records[i] = normalizeModelProviderRecord(records[i])
		return s.saveModelProviderRecordsLocked(records)
	}
	record := normalizeModelProviderRecord(modelProviderRecord{
		ID:           id,
		Name:         providerDisplayName(workspaceID, cfg),
		Type:         "openai-compatible",
		BaseURL:      strings.TrimSpace(cfg.BaseURL),
		APIKey:       strings.TrimSpace(cfg.APIKey),
		DefaultModel: strings.TrimSpace(cfg.Model),
		Models:       normalizeProviderModelNames(cfg.Models, cfg.Model),
		TimeoutMS:    120000,
		Enabled:      strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	records = append(records, record)
	return s.saveModelProviderRecordsLocked(records)
}
