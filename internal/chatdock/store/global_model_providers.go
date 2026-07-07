package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

const (
	modelProvidersMetaKey          = "model_providers_v1"
	modelProviderKeyStrategyAuto   = "auto"
	modelProviderKeyStrategyManual = "manual"
)

type modelProviderAPIKeyRecord struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	APIKey       string     `json:"api_key,omitempty"`
	Enabled      bool       `json:"enabled"`
	Priority     int        `json:"priority"`
	LastStatus   string     `json:"last_status,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type modelProviderRecord struct {
	ID            string                      `json:"id"`
	Name          string                      `json:"name"`
	Type          string                      `json:"type"`
	BaseURL       string                      `json:"base_url"`
	APIKey        string                      `json:"api_key,omitempty"`
	DefaultModel  string                      `json:"default_model"`
	Models        []string                    `json:"models,omitempty"`
	TimeoutMS     int                         `json:"timeout_ms"`
	Enabled       bool                        `json:"enabled"`
	KeyStrategy   string                      `json:"key_strategy,omitempty"`
	SelectedKeyID string                      `json:"selected_key_id,omitempty"`
	APIKeys       []modelProviderAPIKeyRecord `json:"api_keys,omitempty"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
}

type ModelProviderKeyConfig struct {
	KeyID   string
	KeyName string
	Config  model.ModelConfig
}

func (s *Store) EnsureGlobalModelProviders() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureGlobalModelProvidersLocked()
}

func (s *Store) ensureGlobalModelProvidersLocked() error {
	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return err
	}
	if len(records) > 0 {
		return nil
	}

	names, err := s.listWorkspaceIDsLocked()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		names = []string{defaultWorkspaceID}
	}
	now := time.Now()
	for _, workspaceID := range names {
		cfg, err := s.modelConfigForWorkspaceLocked(workspaceID)
		if err != nil {
			return err
		}
		id := providerIDFromWorkspace(workspaceID)
		record := modelProviderRecord{
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
		}
		record = normalizeModelProviderRecord(record)
		records = append(records, record)
		cfg.ProviderID = id
		if err := s.setWorkspaceJSONLocked(workspaceID, "config", cfg); err != nil {
			return err
		}
	}
	return s.saveModelProviderRecordsLocked(records)
}

func (s *Store) ListModelProviders() ([]ModelProvider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return nil, err
	}
	providers := make([]ModelProvider, 0, len(records))
	for _, record := range records {
		providers = append(providers, publicModelProvider(record))
	}
	sort.Slice(providers, func(i, j int) bool {
		if providers[i].Enabled != providers[j].Enabled {
			return providers[i].Enabled
		}
		return providers[i].UpdatedAt.After(providers[j].UpdatedAt)
	})
	return providers, nil
}

func (s *Store) CreateModelProvider(input ModelProviderInput) (ModelProvider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return ModelProvider{}, err
	}
	now := time.Now()
	id := normalizeProviderID(input.ID)
	if id == "" {
		id = normalizeProviderID(input.Name)
	}
	if id == "" {
		id = normalizeProviderID(hostFromURL(input.BaseURL))
	}
	if id == "" {
		id = fmt.Sprintf("provider-%d", now.Unix())
	}
	id = uniqueProviderID(id, records)

	record := modelProviderRecord{
		ID:           id,
		Name:         strings.TrimSpace(input.Name),
		Type:         strings.TrimSpace(input.Type),
		BaseURL:      strings.TrimSpace(input.BaseURL),
		APIKey:       strings.TrimSpace(input.APIKey),
		DefaultModel: strings.TrimSpace(input.DefaultModel),
		Models:       normalizeProviderModelNames(input.Models, input.DefaultModel),
		TimeoutMS:    input.TimeoutMS,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if !input.Enabled && strings.TrimSpace(input.BaseURL) == "" {
		record.Enabled = false
	}
	record.KeyStrategy = input.KeyStrategy
	record.SelectedKeyID = input.SelectedKeyID
	record.APIKeys = inputKeysToRecords(input.APIKeys, nil, input.APIKey, now)
	record = normalizeModelProviderRecord(record)
	if err := validateModelProviderRecord(record); err != nil {
		return ModelProvider{}, err
	}
	records = append(records, record)
	if err := s.saveModelProviderRecordsLocked(records); err != nil {
		return ModelProvider{}, err
	}
	return publicModelProvider(record), nil
}

func (s *Store) UpdateModelProvider(id string, input ModelProviderInput) (ModelProvider, error) {
	id = normalizeProviderID(id)
	if id == "" {
		return ModelProvider{}, fmt.Errorf("model provider id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return ModelProvider{}, err
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		now := time.Now()
		record := records[i]
		record.Name = strings.TrimSpace(input.Name)
		record.Type = strings.TrimSpace(input.Type)
		record.BaseURL = strings.TrimSpace(input.BaseURL)
		if !isMaskedSecret(input.APIKey) {
			record.APIKey = strings.TrimSpace(input.APIKey)
		}
		record.DefaultModel = strings.TrimSpace(input.DefaultModel)
		record.Models = normalizeProviderModelNames(input.Models, input.DefaultModel)
		record.TimeoutMS = input.TimeoutMS
		record.Enabled = input.Enabled
		record.KeyStrategy = strings.TrimSpace(input.KeyStrategy)
		record.SelectedKeyID = normalizeProviderKeyID(input.SelectedKeyID)
		if input.APIKeys != nil {
			record.APIKeys = inputKeysToRecords(input.APIKeys, record.APIKeys, record.APIKey, now)
		} else if !isMaskedSecret(input.APIKey) {
			record.APIKeys = upsertLegacyAPIKeyRecord(record.APIKeys, strings.TrimSpace(input.APIKey), now)
		}
		record.UpdatedAt = now
		record = normalizeModelProviderRecord(record)
		if err := validateModelProviderRecord(record); err != nil {
			return ModelProvider{}, err
		}
		records[i] = record
		if err := s.saveModelProviderRecordsLocked(records); err != nil {
			return ModelProvider{}, err
		}
		return publicModelProvider(record), nil
	}
	return ModelProvider{}, fmt.Errorf("model provider not found: %s", id)
}

func (s *Store) UpdateModelProviderModels(id string, models []string) (ModelProvider, error) {
	id = normalizeProviderID(id)
	if id == "" {
		return ModelProvider{}, fmt.Errorf("model provider id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return ModelProvider{}, err
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		records[i].Models = normalizeProviderModelNames(models, records[i].DefaultModel)
		if records[i].DefaultModel == "" && len(records[i].Models) > 0 {
			records[i].DefaultModel = records[i].Models[0]
		}
		records[i].UpdatedAt = time.Now()
		records[i] = normalizeModelProviderRecord(records[i])
		if err := s.saveModelProviderRecordsLocked(records); err != nil {
			return ModelProvider{}, err
		}
		return publicModelProvider(records[i]), nil
	}
	return ModelProvider{}, fmt.Errorf("model provider not found: %s", id)
}

func (s *Store) MarkModelProviderKeyTestResult(providerID, keyID string, ok bool, errText string, selectOnSuccess bool) (ModelProvider, error) {
	providerID = normalizeProviderID(providerID)
	keyID = normalizeProviderKeyID(keyID)
	if providerID == "" {
		return ModelProvider{}, fmt.Errorf("model provider id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return ModelProvider{}, err
	}
	now := time.Now()
	for i := range records {
		if records[i].ID != providerID {
			continue
		}
		for j := range records[i].APIKeys {
			if records[i].APIKeys[j].ID != keyID {
				continue
			}
			if ok {
				records[i].APIKeys[j].LastStatus = "ok"
				records[i].APIKeys[j].LastError = ""
				if selectOnSuccess {
					records[i].SelectedKeyID = keyID
				}
			} else {
				records[i].APIKeys[j].LastStatus = "error"
				records[i].APIKeys[j].LastError = strings.TrimSpace(errText)
			}
			records[i].APIKeys[j].LastTestedAt = &now
			records[i].APIKeys[j].UpdatedAt = now
			records[i].UpdatedAt = now
			records[i] = normalizeModelProviderRecord(records[i])
			if err := s.saveModelProviderRecordsLocked(records); err != nil {
				return ModelProvider{}, err
			}
			return publicModelProvider(records[i]), nil
		}
		return ModelProvider{}, fmt.Errorf("model provider key not found: %s", keyID)
	}
	return ModelProvider{}, fmt.Errorf("model provider not found: %s", providerID)
}

func (s *Store) DeleteModelProvider(id string) error {
	id = normalizeProviderID(id)
	if id == "" {
		return fmt.Errorf("model provider id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	names, err := s.listWorkspaceIDsLocked()
	if err != nil {
		return err
	}
	for _, workspaceID := range names {
		cfg, err := s.modelConfigForWorkspaceLocked(workspaceID)
		if err != nil {
			return err
		}
		if cfg.ProviderID == id {
			return fmt.Errorf("model provider is used by workspace: %s", workspaceID)
		}
	}

	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return err
	}
	next := records[:0]
	found := false
	for _, record := range records {
		if record.ID == id {
			found = true
			continue
		}
		next = append(next, record)
	}
	if !found {
		return fmt.Errorf("model provider not found: %s", id)
	}
	if len(next) == 0 {
		return fmt.Errorf("last model provider cannot be deleted")
	}
	return s.saveModelProviderRecordsLocked(next)
}

func (s *Store) ModelProviderConfig(id string) (model.ModelConfig, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelProviderConfigLocked(id)
}

func (s *Store) modelProviderConfigLocked(id string) (model.ModelConfig, bool, error) {
	id = normalizeProviderID(id)
	if id == "" {
		return model.ModelConfig{}, false, nil
	}
	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return model.ModelConfig{}, false, err
	}
	for _, record := range records {
		if record.ID != id {
			continue
		}
		apiKey, err := selectedAPIKeyForRecord(record)
		if err != nil {
			return model.ModelConfig{}, true, err
		}
		return model.NormalizeModelConfig(model.ModelConfig{
			ProviderID: record.ID,
			BaseURL:    record.BaseURL,
			APIKey:     apiKey,
			Model:      record.DefaultModel,
			Models:     append([]string(nil), record.Models...),
		}), true, nil
	}
	return model.ModelConfig{}, false, nil
}

func (s *Store) ModelProviderKeyConfigs(id, selectedKeyID, modelName string, allKeys bool) ([]ModelProviderKeyConfig, ModelProvider, error) {
	id = normalizeProviderID(id)
	selectedKeyID = normalizeProviderKeyID(selectedKeyID)
	if id == "" {
		return nil, ModelProvider{}, fmt.Errorf("model provider id is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return nil, ModelProvider{}, err
	}
	for _, record := range records {
		if record.ID != id {
			continue
		}
		modelForTest := strings.TrimSpace(modelName)
		if modelForTest == "" {
			modelForTest = record.DefaultModel
		}
		makeCfg := func(key modelProviderAPIKeyRecord) ModelProviderKeyConfig {
			return ModelProviderKeyConfig{KeyID: key.ID, KeyName: key.Name, Config: model.NormalizeModelConfig(model.ModelConfig{ProviderID: record.ID, BaseURL: record.BaseURL, APIKey: key.APIKey, Model: modelForTest, Models: append([]string(nil), record.Models...)})}
		}
		if len(record.APIKeys) == 0 {
			return []ModelProviderKeyConfig{{Config: model.NormalizeModelConfig(model.ModelConfig{ProviderID: record.ID, BaseURL: record.BaseURL, APIKey: strings.TrimSpace(record.APIKey), Model: modelForTest, Models: append([]string(nil), record.Models...)})}}, publicModelProvider(record), nil
		}
		if selectedKeyID != "" {
			for _, key := range record.APIKeys {
				if key.ID == selectedKeyID {
					if !key.Enabled || strings.TrimSpace(key.APIKey) == "" {
						return nil, ModelProvider{}, fmt.Errorf("model provider key is disabled or empty: %s", selectedKeyID)
					}
					return []ModelProviderKeyConfig{makeCfg(key)}, publicModelProvider(record), nil
				}
			}
			return nil, ModelProvider{}, fmt.Errorf("model provider key not found: %s", selectedKeyID)
		}
		if allKeys {
			keys := sortedProviderKeys(record.APIKeys)
			out := make([]ModelProviderKeyConfig, 0, len(keys))
			for _, key := range keys {
				if !key.Enabled || strings.TrimSpace(key.APIKey) == "" {
					continue
				}
				out = append(out, makeCfg(key))
			}
			if len(out) == 0 {
				return nil, ModelProvider{}, fmt.Errorf("model provider has no enabled api keys: %s", record.ID)
			}
			return out, publicModelProvider(record), nil
		}
		key, ok, err := selectedAPIKeyRecordForRecord(record)
		if err != nil {
			return nil, ModelProvider{}, err
		}
		if !ok {
			return []ModelProviderKeyConfig{{Config: model.NormalizeModelConfig(model.ModelConfig{ProviderID: record.ID, BaseURL: record.BaseURL, APIKey: "", Model: modelForTest, Models: append([]string(nil), record.Models...)})}}, publicModelProvider(record), nil
		}
		return []ModelProviderKeyConfig{makeCfg(key)}, publicModelProvider(record), nil
	}
	return nil, ModelProvider{}, fmt.Errorf("model provider not found: %s", id)
}
