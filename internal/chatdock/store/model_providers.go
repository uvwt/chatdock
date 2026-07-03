package store

import (
	"encoding/json"
	"strings"

	"chatdock/internal/chatdock/model"
)

func (s *Store) ListModelProviders() ([]ModelProvider, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	providers := make([]ModelProvider, 0, len(workspaces.Workspaces))
	for _, ws := range workspaces.Workspaces {
		cfg, err := s.modelConfigForPrompt(ws.ID)
		if err != nil {
			return nil, err
		}
		providers = append(providers, ModelProvider{
			ID:            cfg.ProviderID,
			Name:          providerName(ws.Name, cfg),
			Type:          "openai-compatible",
			BaseURL:       cfg.BaseURL,
			HasAPIKey:     strings.TrimSpace(cfg.APIKey) != "",
			APIKeyMasked:  maskSecret(cfg.APIKey),
			DefaultModel:  cfg.Model,
			TimeoutMS:     120000,
			Enabled:       strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "",
			WorkspaceID:   ws.ID,
			WorkspaceName: ws.Name,
			CreatedAt:     ws.CreatedAt,
			UpdatedAt:     ws.UpdatedAt,
		})
	}
	return providers, nil
}

func (s *Store) modelConfigForPrompt(prompt string) (model.ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelConfigForPromptLocked(prompt)
}

func (s *Store) modelConfigForPromptLocked(prompt string) (model.ModelConfig, error) {
	raw, ok, err := s.getPromptRawLocked(prompt, "config")
	if err != nil {
		return model.ModelConfig{}, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return model.DefaultModelConfig(), nil
	}
	var cfg model.ModelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return model.ModelConfig{}, err
	}
	return model.NormalizeModelConfig(cfg), nil
}

func providerName(workspace string, cfg model.ModelConfig) string {
	base := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cfg.BaseURL), "https://"), "http://")
	if base == "" {
		base = "OpenAI Compatible"
	}
	return workspace + " · " + base
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "******"
	}
	return value[:4] + "******" + value[len(value)-4:]
}
