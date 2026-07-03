package store

import (
	"encoding/json"
	"fmt"
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
			Models:        append([]string(nil), cfg.Models...),
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

func (s *Store) ResolveChatModelConfig(base model.ModelConfig, providerID string, selectedModel string) (model.ModelConfig, error) {
	providerID = strings.TrimSpace(providerID)
	selectedModel = strings.TrimSpace(selectedModel)
	if providerID == "" && selectedModel == "" {
		return model.NormalizeModelConfig(base), nil
	}

	next := model.NormalizeModelConfig(base)
	if providerID != "" {
		s.mu.RLock()
		exists, err := s.promptExistsLocked(providerID)
		if err != nil {
			s.mu.RUnlock()
			return model.ModelConfig{}, err
		}
		if !exists {
			s.mu.RUnlock()
			return model.ModelConfig{}, fmt.Errorf("model provider not found: %s", providerID)
		}
		providerCfg, err := s.modelConfigForPromptLocked(providerID)
		s.mu.RUnlock()
		if err != nil {
			return model.ModelConfig{}, err
		}
		// 供应商选择只切换连接、密钥和模型；当前会话的系统提示词、技能和上下文策略继续沿用当前工作空间。
		next.ProviderID = providerCfg.ProviderID
		next.BaseURL = providerCfg.BaseURL
		next.APIKey = providerCfg.APIKey
		next.Models = append([]string(nil), providerCfg.Models...)
		if selectedModel == "" {
			selectedModel = providerCfg.Model
		}
	}
	if selectedModel != "" {
		next.Model = selectedModel
	}
	next = model.NormalizeModelConfig(next)
	if strings.TrimSpace(next.BaseURL) == "" || strings.TrimSpace(next.Model) == "" {
		return model.ModelConfig{}, fmt.Errorf("model provider is incomplete")
	}
	return next, nil
}
