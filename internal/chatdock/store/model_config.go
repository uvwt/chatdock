package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) ModelConfig() (model.ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelConfigLocked()
}

func (s *Store) BestEffortModelConfig() model.ModelConfig {
	cfg, err := s.ModelConfig()
	if err != nil {
		return model.DefaultModelConfig()
	}
	return cfg
}

func (s *Store) SaveModelConfig(next model.ModelConfig) (model.ModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveModelConfigLocked(next)
}

func (s *Store) saveModelConfigLocked(next model.ModelConfig) (model.ModelConfig, error) {
	current, err := s.modelConfigLocked()
	if err != nil {
		return model.ModelConfig{}, err
	}
	requestedBaseURL := strings.TrimSpace(next.BaseURL)
	requestedProviderID := strings.TrimSpace(next.ProviderID)
	if requestedProviderID == "" {
		if requestedBaseURL != "" {
			next.ProviderID = "provider_default"
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
		if err := deleteToolEmbeddingsWith(tx); err != nil {
			return model.ModelConfig{}, err
		}
	}
	if requestedBaseURL != "" {
		if err := upsertProviderFromConfigWith(tx, next); err != nil {
			return model.ModelConfig{}, err
		}
	}
	merged, err := applyProviderToConfigWith(tx, next)
	if err != nil {
		return model.ModelConfig{}, err
	}
	if err := normalizeFallbackModelSelectionWith(tx, &merged); err != nil {
		return model.ModelConfig{}, err
	}
	if err := setGlobalJSONWith(tx, "config", merged, time.Now()); err != nil {
		return model.ModelConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ModelConfig{}, err
	}
	return merged, nil
}

func normalizeFallbackModelSelectionWith(reader sqlQueryer, cfg *model.ModelConfig) error {
	providerID := normalizeProviderID(cfg.FallbackProviderID)
	if providerID == "" {
		cfg.FallbackProviderID = ""
		cfg.FallbackModel = ""
		return nil
	}

	providerCfg, ok, err := modelProviderConfigWith(reader, providerID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("fallback model provider not found: %s", providerID)
	}
	modelName := strings.TrimSpace(cfg.FallbackModel)
	if modelName == "" {
		modelName = providerCfg.Model
	}
	if modelName == "" {
		return fmt.Errorf("fallback model is required")
	}
	if providerCfg.ProviderID == cfg.ProviderID && modelName == cfg.Model {
		return fmt.Errorf("fallback model must differ from primary model")
	}
	cfg.FallbackProviderID = providerCfg.ProviderID
	cfg.FallbackModel = modelName
	return nil
}

func upsertProviderFromConfigWith(db sqlQueryWriter, cfg model.ModelConfig) error {
	records, err := loadModelProviderRecordsWith(db)
	if err != nil {
		return err
	}
	now := time.Now()
	id := normalizeProviderID(cfg.ProviderID)
	if id == "" {
		id = "provider_default"
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
	record := normalizeModelProviderRecord(modelProviderRecord{ID: id, Name: providerDisplayName(cfg), Type: "openai-compatible", BaseURL: strings.TrimSpace(cfg.BaseURL), APIKeys: upsertProviderAPIKey(nil, "", cfg.APIKey, now), DefaultModel: strings.TrimSpace(cfg.Model), Models: normalizeProviderModelNames(cfg.Models, cfg.Model), TimeoutMS: 120000, Enabled: strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "", CreatedAt: now, UpdatedAt: now})
	records = append(records, record)
	return saveModelProviderRecordsWith(db, records)
}
