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

const modelProvidersMetaKey = "model_providers_v1"

type modelProviderRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	BaseURL      string    `json:"base_url"`
	APIKey       string    `json:"api_key,omitempty"`
	DefaultModel string    `json:"default_model"`
	Models       []string  `json:"models,omitempty"`
	TimeoutMS    int       `json:"timeout_ms"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
		record.UpdatedAt = time.Now()
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
		return model.NormalizeModelConfig(model.ModelConfig{
			ProviderID: record.ID,
			BaseURL:    record.BaseURL,
			APIKey:     record.APIKey,
			Model:      record.DefaultModel,
			Models:     append([]string(nil), record.Models...),
		}), true, nil
	}
	return model.ModelConfig{}, false, nil
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
	return ModelProvider{
		ID:           record.ID,
		Name:         record.Name,
		Type:         record.Type,
		BaseURL:      record.BaseURL,
		HasAPIKey:    strings.TrimSpace(record.APIKey) != "",
		APIKeyMasked: maskSecret(record.APIKey),
		DefaultModel: record.DefaultModel,
		Models:       append([]string(nil), record.Models...),
		TimeoutMS:    record.TimeoutMS,
		Enabled:      record.Enabled,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
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
	record.ID = normalizeProviderID(record.ID)
	record.Name = strings.TrimSpace(record.Name)
	record.Type = strings.TrimSpace(record.Type)
	record.BaseURL = strings.TrimSpace(record.BaseURL)
	record.APIKey = strings.TrimSpace(record.APIKey)
	record.DefaultModel = strings.TrimSpace(record.DefaultModel)
	record.Models = normalizeProviderModelNames(record.Models, record.DefaultModel)
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
		record.CreatedAt = time.Now()
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
	return nil
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
