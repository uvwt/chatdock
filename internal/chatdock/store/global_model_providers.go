package store

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

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

	names, err := s.listPromptNamesLocked()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		names = []string{defaultPromptName}
	}
	now := time.Now()
	for _, workspaceID := range names {
		cfg, err := s.modelConfigForPromptLocked(workspaceID)
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
		if err := s.setPromptJSONLocked(workspaceID, "config", cfg); err != nil {
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

	names, err := s.listPromptNamesLocked()
	if err != nil {
		return err
	}
	for _, workspaceID := range names {
		cfg, err := s.modelConfigForPromptLocked(workspaceID)
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

func (s *Store) loadModelProviderRecordsLocked() ([]modelProviderRecord, error) {
	raw, err := s.metaValue(modelProvidersMetaKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var records []modelProviderRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, err
	}
	out := make([]modelProviderRecord, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		record = normalizeModelProviderRecord(record)
		if record.ID == "" || seen[record.ID] {
			continue
		}
		seen[record.ID] = true
		out = append(out, record)
	}
	return out, nil
}

func (s *Store) saveModelProviderRecordsLocked(records []modelProviderRecord) error {
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return s.setMetaValue(modelProvidersMetaKey, string(raw))
}

func publicModelProvider(record modelProviderRecord) ModelProvider {
	apiKeys := make([]ModelProviderAPIKey, 0, len(record.APIKeys))
	for _, key := range sortedProviderKeys(record.APIKeys) {
		apiKeys = append(apiKeys, ModelProviderAPIKey{
			ID:           key.ID,
			Name:         key.Name,
			HasAPIKey:    strings.TrimSpace(key.APIKey) != "",
			APIKeyMasked: maskSecret(key.APIKey),
			Enabled:      key.Enabled,
			Priority:     key.Priority,
			LastStatus:   key.LastStatus,
			LastError:    key.LastError,
			LastTestedAt: key.LastTestedAt,
			CreatedAt:    key.CreatedAt,
			UpdatedAt:    key.UpdatedAt,
		})
	}
	selectedMasked := ""
	if key, ok := publicSelectedKey(record); ok {
		selectedMasked = maskSecret(key.APIKey)
	} else {
		selectedMasked = maskSecret(record.APIKey)
	}
	return ModelProvider{
		ID:            record.ID,
		Name:          record.Name,
		Type:          record.Type,
		BaseURL:       record.BaseURL,
		HasAPIKey:     providerHasAnyAPIKey(record),
		APIKeyMasked:  selectedMasked,
		DefaultModel:  record.DefaultModel,
		Models:        append([]string(nil), record.Models...),
		TimeoutMS:     record.TimeoutMS,
		Enabled:       record.Enabled,
		KeyStrategy:   record.KeyStrategy,
		SelectedKeyID: record.SelectedKeyID,
		APIKeys:       apiKeys,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}

func providerIDFromWorkspace(workspace string) string {
	id := normalizeProviderID(workspace)
	if id == "" {
		return defaultPromptName
	}
	return id
}

func providerDisplayName(workspace string, cfg model.ModelConfig) string {
	name := strings.TrimSpace(workspace)
	host := hostFromURL(cfg.BaseURL)
	if host == "" {
		host = "OpenAI Compatible"
	}
	if name == "" {
		return host
	}
	return name + " · " + host
}

func normalizeModelProviderRecord(record modelProviderRecord) modelProviderRecord {
	now := time.Now()
	record.ID = normalizeProviderID(record.ID)
	record.Name = strings.TrimSpace(record.Name)
	record.Type = strings.TrimSpace(record.Type)
	record.BaseURL = strings.TrimSpace(record.BaseURL)
	record.APIKey = strings.TrimSpace(record.APIKey)
	record.DefaultModel = strings.TrimSpace(record.DefaultModel)
	record.Models = normalizeProviderModelNames(record.Models, record.DefaultModel)
	record.KeyStrategy = normalizeProviderKeyStrategy(record.KeyStrategy)
	record.SelectedKeyID = normalizeProviderKeyID(record.SelectedKeyID)
	if len(record.APIKeys) == 0 && record.APIKey != "" {
		record.APIKeys = []modelProviderAPIKeyRecord{{ID: "main", Name: "主 key", APIKey: record.APIKey, Enabled: true, Priority: 1, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}}
	}
	record.APIKeys = normalizeProviderAPIKeyRecords(record.APIKeys, now)
	if record.SelectedKeyID == "" {
		if key, ok := firstEnabledProviderKey(record.APIKeys); ok {
			record.SelectedKeyID = key.ID
		}
	}
	if selected, ok := keyByID(record.APIKeys, record.SelectedKeyID); ok {
		record.APIKey = selected.APIKey
	} else if key, ok := firstEnabledProviderKey(record.APIKeys); ok {
		record.APIKey = key.APIKey
	}
	if record.Type == "" {
		record.Type = "openai-compatible"
	}
	if record.Name == "" {
		record.Name = record.ID
	}
	if record.TimeoutMS <= 0 {
		record.TimeoutMS = 120000
	}
	if record.DefaultModel == "" && len(record.Models) > 0 {
		record.DefaultModel = record.Models[0]
	}
	if len(record.Models) == 0 && record.DefaultModel != "" {
		record.Models = []string{record.DefaultModel}
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	return record
}

func validateModelProviderRecord(record modelProviderRecord) error {
	if record.ID == "" {
		return fmt.Errorf("model provider id is required")
	}
	if strings.TrimSpace(record.BaseURL) == "" {
		return fmt.Errorf("model provider base_url is required")
	}
	if strings.TrimSpace(record.DefaultModel) == "" {
		return fmt.Errorf("model provider default_model is required")
	}
	if record.KeyStrategy == modelProviderKeyStrategyManual {
		if record.SelectedKeyID == "" && len(record.APIKeys) > 0 {
			return fmt.Errorf("selected_key_id is required when key_strategy is manual")
		}
		if record.SelectedKeyID != "" {
			if _, ok := keyByID(record.APIKeys, record.SelectedKeyID); !ok {
				return fmt.Errorf("selected api key not found: %s", record.SelectedKeyID)
			}
		}
	}
	return nil
}

func inputKeysToRecords(inputs []ModelProviderAPIKeyInput, previous []modelProviderAPIKeyRecord, legacyAPIKey string, now time.Time) []modelProviderAPIKeyRecord {
	previousByID := map[string]modelProviderAPIKeyRecord{}
	for _, item := range previous {
		previousByID[item.ID] = item
	}
	out := make([]modelProviderAPIKeyRecord, 0, len(inputs))
	used := map[string]bool{}
	for idx, input := range inputs {
		id := normalizeProviderKeyID(input.ID)
		if id == "" {
			id = normalizeProviderKeyID(input.Name)
		}
		if id == "" {
			id = fmt.Sprintf("key-%d", idx+1)
		}
		base, exists := previousByID[id]
		if !exists {
			base = modelProviderAPIKeyRecord{ID: id, Enabled: true, Priority: idx + 1, CreatedAt: now, UpdatedAt: now}
		}
		if used[id] {
			id = uniqueProviderKeyID(id, used)
			base.ID = id
		}
		used[id] = true
		if strings.TrimSpace(input.Name) != "" {
			base.Name = strings.TrimSpace(input.Name)
		}
		if !isMaskedSecret(input.APIKey) {
			base.APIKey = strings.TrimSpace(input.APIKey)
		}
		if input.Enabled != nil {
			base.Enabled = *input.Enabled
		}
		if input.Priority > 0 {
			base.Priority = input.Priority
		} else if base.Priority <= 0 {
			base.Priority = idx + 1
		}
		base.UpdatedAt = now
		out = append(out, base)
	}
	if len(out) == 0 && !isMaskedSecret(legacyAPIKey) {
		out = append(out, modelProviderAPIKeyRecord{ID: "main", Name: "主 key", APIKey: strings.TrimSpace(legacyAPIKey), Enabled: true, Priority: 1, CreatedAt: now, UpdatedAt: now})
	}
	return out
}

func upsertLegacyAPIKeyRecord(keys []modelProviderAPIKeyRecord, apiKey string, now time.Time) []modelProviderAPIKeyRecord {
	if strings.TrimSpace(apiKey) == "" {
		return keys
	}
	if len(keys) == 0 {
		return []modelProviderAPIKeyRecord{{ID: "main", Name: "主 key", APIKey: strings.TrimSpace(apiKey), Enabled: true, Priority: 1, CreatedAt: now, UpdatedAt: now}}
	}
	selected := keys[0].ID
	for _, key := range keys {
		if key.Enabled {
			selected = key.ID
			break
		}
	}
	for i := range keys {
		if keys[i].ID == selected {
			keys[i].APIKey = strings.TrimSpace(apiKey)
			keys[i].UpdatedAt = now
			return keys
		}
	}
	return keys
}

func normalizeProviderAPIKeyRecords(keys []modelProviderAPIKeyRecord, now time.Time) []modelProviderAPIKeyRecord {
	out := make([]modelProviderAPIKeyRecord, 0, len(keys))
	used := map[string]bool{}
	for idx, key := range keys {
		key.ID = normalizeProviderKeyID(key.ID)
		if key.ID == "" {
			key.ID = fmt.Sprintf("key-%d", idx+1)
		}
		if used[key.ID] {
			key.ID = uniqueProviderKeyID(key.ID, used)
		}
		used[key.ID] = true
		key.Name = strings.TrimSpace(key.Name)
		key.APIKey = strings.TrimSpace(key.APIKey)
		if key.Name == "" {
			key.Name = key.ID
		}
		if key.Priority <= 0 {
			key.Priority = idx + 1
		}
		if key.CreatedAt.IsZero() {
			key.CreatedAt = now
		}
		if key.UpdatedAt.IsZero() {
			key.UpdatedAt = key.CreatedAt
		}
		out = append(out, key)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func selectedAPIKeyForRecord(record modelProviderRecord) (string, error) {
	key, ok, err := selectedAPIKeyRecordForRecord(record)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(key.APIKey), nil
}

func selectedAPIKeyRecordForRecord(record modelProviderRecord) (modelProviderAPIKeyRecord, bool, error) {
	keys := sortedProviderKeys(record.APIKeys)
	if len(keys) == 0 {
		return modelProviderAPIKeyRecord{}, false, nil
	}
	if record.KeyStrategy == modelProviderKeyStrategyManual {
		if record.SelectedKeyID == "" {
			return modelProviderAPIKeyRecord{}, false, fmt.Errorf("model provider key_strategy=manual requires selected_key_id: %s", record.ID)
		}
		key, ok := keyByID(keys, record.SelectedKeyID)
		if !ok {
			return modelProviderAPIKeyRecord{}, false, fmt.Errorf("selected api key not found: %s", record.SelectedKeyID)
		}
		if !key.Enabled || strings.TrimSpace(key.APIKey) == "" {
			return modelProviderAPIKeyRecord{}, false, fmt.Errorf("selected api key is disabled or empty: %s", record.SelectedKeyID)
		}
		return key, true, nil
	}
	if record.SelectedKeyID != "" {
		if key, ok := keyByID(keys, record.SelectedKeyID); ok && key.Enabled && strings.TrimSpace(key.APIKey) != "" {
			return key, true, nil
		}
	}
	for _, key := range keys {
		if key.Enabled && strings.TrimSpace(key.APIKey) != "" {
			return key, true, nil
		}
	}
	return modelProviderAPIKeyRecord{}, false, fmt.Errorf("model provider has no enabled api keys: %s", record.ID)
}

func publicSelectedKey(record modelProviderRecord) (modelProviderAPIKeyRecord, bool) {
	if record.SelectedKeyID != "" {
		if key, ok := keyByID(record.APIKeys, record.SelectedKeyID); ok {
			return key, true
		}
	}
	return firstEnabledProviderKey(record.APIKeys)
}

func providerHasAnyAPIKey(record modelProviderRecord) bool {
	if strings.TrimSpace(record.APIKey) != "" {
		return true
	}
	for _, key := range record.APIKeys {
		if strings.TrimSpace(key.APIKey) != "" {
			return true
		}
	}
	return false
}

func firstEnabledProviderKey(keys []modelProviderAPIKeyRecord) (modelProviderAPIKeyRecord, bool) {
	for _, key := range sortedProviderKeys(keys) {
		if key.Enabled && strings.TrimSpace(key.APIKey) != "" {
			return key, true
		}
	}
	return modelProviderAPIKeyRecord{}, false
}

func keyByID(keys []modelProviderAPIKeyRecord, id string) (modelProviderAPIKeyRecord, bool) {
	id = normalizeProviderKeyID(id)
	if id == "" {
		return modelProviderAPIKeyRecord{}, false
	}
	for _, key := range keys {
		if key.ID == id {
			return key, true
		}
	}
	return modelProviderAPIKeyRecord{}, false
}

func sortedProviderKeys(keys []modelProviderAPIKeyRecord) []modelProviderAPIKeyRecord {
	out := append([]modelProviderAPIKeyRecord(nil), keys...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func normalizeProviderKeyStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case modelProviderKeyStrategyManual:
		return modelProviderKeyStrategyManual
	default:
		return modelProviderKeyStrategyAuto
	}
}

func normalizeProviderID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '_' || r == '-' {
			if !lastDash {
				b.WriteRune(r)
				lastDash = r == '-'
			}
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-_")
}

func normalizeProviderKeyID(value string) string {
	return normalizeProviderID(value)
}

func uniqueProviderID(id string, records []modelProviderRecord) string {
	used := map[string]bool{}
	for _, record := range records {
		used[record.ID] = true
	}
	if !used[id] {
		return id
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", id, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func uniqueProviderKeyID(id string, used map[string]bool) string {
	base := id
	if base == "" {
		base = "key"
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func normalizeProviderModelNames(models []string, fallback string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	add(fallback)
	for _, name := range models {
		add(name)
	}
	return out
}

func hostFromURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://")
	}
	return parsed.Host
}

func isMaskedSecret(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	trimmed := strings.Trim(value, "*")
	return trimmed == ""
}
