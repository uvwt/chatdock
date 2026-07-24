package store

import (
	"chatdock/internal/chatdock/modelprovider"
	"fmt"
	"sort"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type ModelProviderKeyConfig = modelprovider.KeyConfig

func (s *Store) EnsureGlobalModelProviders() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureGlobalModelProvidersLocked()
}

func (s *Store) ensureGlobalModelProvidersLocked() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	records, err := modelprovider.LoadRecords(tx)
	if err != nil {
		return err
	}
	if len(records) > 0 {
		return nil
	}

	cfg, err := modelConfigWith(tx)
	if err != nil {
		return err
	}
	now := time.Now()
	id := modelprovider.NormalizeID(cfg.ProviderID)
	if id == "" {
		id = "provider_default"
	}
	record := modelprovider.NormalizeRecord(modelprovider.Record{
		ID:           id,
		Name:         modelprovider.DisplayName(cfg),
		Type:         "openai-compatible",
		BaseURL:      strings.TrimSpace(cfg.BaseURL),
		APIKeys:      modelprovider.UpsertAPIKey(nil, "", cfg.APIKey, now),
		DefaultModel: strings.TrimSpace(cfg.Model),
		Models:       modelprovider.NormalizeModelNames(cfg.Models, cfg.Model),
		TimeoutMS:    120000,
		Enabled:      strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	records = append(records, record)
	cfg.ProviderID = id
	if err := setGlobalJSONWith(tx, "config", cfg, now); err != nil {
		return err
	}
	if err := modelprovider.SaveRecords(tx, records); err != nil {
		return err
	}
	return tx.Commit()
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
		providers = append(providers, modelprovider.Public(record))
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
	records, record, err := modelprovider.CreateRecord(records, input, time.Now())
	if err != nil {
		return ModelProvider{}, err
	}
	if err := s.saveModelProviderRecordsLocked(records); err != nil {
		return ModelProvider{}, err
	}
	return modelprovider.Public(record), nil
}

func (s *Store) UpdateModelProvider(id string, input ModelProviderInput) (ModelProvider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.loadModelProviderRecordsLocked()
	if err != nil {
		return ModelProvider{}, err
	}
	records, record, err := modelprovider.UpdateRecord(records, id, input, time.Now())
	if err != nil {
		return ModelProvider{}, err
	}
	if err := s.saveModelProviderRecordsLocked(records); err != nil {
		return ModelProvider{}, err
	}
	return modelprovider.Public(record), nil
}

func (s *Store) UpsertModelProvider(id string, input ModelProviderInput, setDefault bool, selectedModel string) (ModelProvider, *model.ModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return ModelProvider{}, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	records, err := modelprovider.LoadRecords(tx)
	if err != nil {
		return ModelProvider{}, nil, err
	}

	id = modelprovider.NormalizeID(id)
	var record modelprovider.Record
	if modelprovider.RecordExists(records, id) {
		records, record, err = modelprovider.UpdateRecord(records, id, input, time.Now())
	} else {
		if id != "" {
			input.ID = id
		}
		records, record, err = modelprovider.CreateRecord(records, input, time.Now())
	}
	if err != nil {
		return ModelProvider{}, nil, err
	}
	if err := modelprovider.SaveRecords(tx, records); err != nil {
		return ModelProvider{}, nil, err
	}

	var savedConfig *model.ModelConfig
	if setDefault {
		if !record.Enabled {
			return ModelProvider{}, nil, fmt.Errorf("model provider is disabled: %s", record.ID)
		}
		cfg, err := modelConfigWith(tx)
		if err != nil {
			return ModelProvider{}, nil, err
		}
		cfg.ProviderID = record.ID
		cfg.Model = strings.TrimSpace(selectedModel)
		if cfg.Model == "" {
			cfg.Model = record.DefaultModel
		}
		cfg.Models = append([]string(nil), record.Models...)
		cfg, err = applyProviderToConfigWith(tx, cfg)
		if err != nil {
			return ModelProvider{}, nil, err
		}
		if err := setGlobalJSONWith(tx, "config", cfg, time.Now()); err != nil {
			return ModelProvider{}, nil, err
		}
		savedConfig = &cfg
	}
	if err := tx.Commit(); err != nil {
		return ModelProvider{}, nil, err
	}
	return modelprovider.Public(record), savedConfig, nil
}

func (s *Store) MarkModelProviderKeyTestResult(providerID, keyID string, ok bool, errText string, selectOnSuccess bool) (ModelProvider, error) {
	providerID = modelprovider.NormalizeID(providerID)
	keyID = modelprovider.NormalizeKeyID(keyID)
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
			records[i] = modelprovider.NormalizeRecord(records[i])
			if err := s.saveModelProviderRecordsLocked(records); err != nil {
				return ModelProvider{}, err
			}
			return modelprovider.Public(records[i]), nil
		}
		return ModelProvider{}, fmt.Errorf("model provider key not found: %s", keyID)
	}
	return ModelProvider{}, fmt.Errorf("model provider not found: %s", providerID)
}

func (s *Store) DeleteModelProvider(id string) error {
	id = modelprovider.NormalizeID(id)
	if id == "" {
		return fmt.Errorf("model provider id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.modelConfigLocked()
	if err != nil {
		return err
	}
	if cfg.ProviderID == id {
		return fmt.Errorf("model provider is used by global config: %s", id)
	}
	if cfg.FallbackProviderID == id {
		return fmt.Errorf("model provider is used as fallback by global config: %s", id)
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
	return modelProviderConfigWith(s.db, id)
}

func modelProviderConfigWith(reader sqlQueryer, id string) (model.ModelConfig, bool, error) {
	id = modelprovider.NormalizeID(id)
	if id == "" {
		return model.ModelConfig{}, false, nil
	}
	records, err := modelprovider.LoadRecords(reader)
	if err != nil {
		return model.ModelConfig{}, false, err
	}
	for _, record := range records {
		if record.ID != id {
			continue
		}
		apiKey, err := modelprovider.SelectedAPIKey(record)
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

func (s *Store) ModelProviderKeyConfigs(id, selectedKeyID, modelName string, allKeys bool) ([]modelprovider.KeyConfig, ModelProvider, error) {
	id = modelprovider.NormalizeID(id)
	selectedKeyID = modelprovider.NormalizeKeyID(selectedKeyID)
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
		makeCfg := func(key modelprovider.APIKeyRecord) modelprovider.KeyConfig {
			return modelprovider.KeyConfig{KeyID: key.ID, KeyName: key.Name, Config: model.NormalizeModelConfig(model.ModelConfig{ProviderID: record.ID, BaseURL: record.BaseURL, APIKey: key.APIKey, Model: modelForTest, Models: append([]string(nil), record.Models...)})}
		}
		if len(record.APIKeys) == 0 {
			return []modelprovider.KeyConfig{{Config: model.NormalizeModelConfig(model.ModelConfig{ProviderID: record.ID, BaseURL: record.BaseURL, Model: modelForTest, Models: append([]string(nil), record.Models...)})}}, modelprovider.Public(record), nil
		}
		if selectedKeyID != "" {
			for _, key := range record.APIKeys {
				if key.ID == selectedKeyID {
					if !key.Enabled || strings.TrimSpace(key.APIKey) == "" {
						return nil, ModelProvider{}, fmt.Errorf("model provider key is disabled or empty: %s", selectedKeyID)
					}
					return []modelprovider.KeyConfig{makeCfg(key)}, modelprovider.Public(record), nil
				}
			}
			return nil, ModelProvider{}, fmt.Errorf("model provider key not found: %s", selectedKeyID)
		}
		if allKeys {
			keys := modelprovider.SortedKeys(record.APIKeys)
			out := make([]modelprovider.KeyConfig, 0, len(keys))
			for _, key := range keys {
				if !key.Enabled || strings.TrimSpace(key.APIKey) == "" {
					continue
				}
				out = append(out, makeCfg(key))
			}
			if len(out) == 0 {
				return nil, ModelProvider{}, fmt.Errorf("model provider has no enabled api keys: %s", record.ID)
			}
			return out, modelprovider.Public(record), nil
		}
		key, ok, err := modelprovider.SelectedAPIKeyRecord(record)
		if err != nil {
			return nil, ModelProvider{}, err
		}
		if !ok {
			return []modelprovider.KeyConfig{{Config: model.NormalizeModelConfig(model.ModelConfig{ProviderID: record.ID, BaseURL: record.BaseURL, APIKey: "", Model: modelForTest, Models: append([]string(nil), record.Models...)})}}, modelprovider.Public(record), nil
		}
		return []modelprovider.KeyConfig{makeCfg(key)}, modelprovider.Public(record), nil
	}
	return nil, ModelProvider{}, fmt.Errorf("model provider not found: %s", id)
}
