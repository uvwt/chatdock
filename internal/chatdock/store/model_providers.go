package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/model"
)

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
	cfg = model.NormalizeModelConfig(cfg)
	return s.applyProviderToConfigLocked(cfg)
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 8 {
		if len(runes) <= 2 {
			return strings.Repeat("*", len(runes))
		}
		return string(runes[:1]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1:])
	}
	prefixLen := 6
	suffixLen := 4
	if len(runes) > 24 {
		prefixLen = 8
		suffixLen = 6
	}
	if len(runes) <= prefixLen+suffixLen {
		return string(runes[:2]) + strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-2:])
	}
	return string(runes[:prefixLen]) + strings.Repeat("*", 8) + string(runes[len(runes)-suffixLen:])
}

func (s *Store) ResolveChatModelConfig(base model.ModelConfig, providerID string, selectedModel string) (model.ModelConfig, error) {
	providerID = normalizeProviderID(providerID)
	selectedModel = strings.TrimSpace(selectedModel)

	s.mu.RLock()
	defer s.mu.RUnlock()

	next := model.NormalizeModelConfig(base)
	if providerID == "" {
		providerID = next.ProviderID
	}
	if providerID != "" {
		providerCfg, ok, err := s.modelProviderConfigLocked(providerID)
		if err != nil {
			return model.ModelConfig{}, err
		}
		if !ok {
			return model.ModelConfig{}, fmt.Errorf("model provider not found: %s", providerID)
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

func (s *Store) applyProviderToConfigLocked(cfg model.ModelConfig) (model.ModelConfig, error) {
	cfg = model.NormalizeModelConfig(cfg)
	providerCfg, ok, err := s.modelProviderConfigLocked(cfg.ProviderID)
	if err != nil {
		return model.ModelConfig{}, err
	}
	if !ok {
		return cfg, nil
	}
	cfg.ProviderID = providerCfg.ProviderID
	cfg.BaseURL = providerCfg.BaseURL
	cfg.APIKey = providerCfg.APIKey
	cfg.Models = append([]string(nil), providerCfg.Models...)
	if cfg.Model == "" {
		cfg.Model = providerCfg.Model
	}
	return model.NormalizeModelConfig(cfg), nil
}
