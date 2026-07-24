package store

import (
	"chatdock/internal/modelprovider"
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/model"
)

func (s *Store) modelConfigLocked() (model.ModelConfig, error) {
	return modelConfigWith(s.db)
}

func modelConfigWith(reader sqlQueryer) (model.ModelConfig, error) {
	raw, ok, err := getGlobalRawWith(reader, "config")
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
	return applyProviderToConfigWith(reader, cfg)
}

func (s *Store) resolveChatModelConfigLocked(base model.ModelConfig, providerID string, selectedModel string) (model.ModelConfig, error) {
	providerID = modelprovider.NormalizeID(providerID)
	selectedModel = strings.TrimSpace(selectedModel)
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
			return model.ModelConfig{}, invalidChatRequest("model provider not found: %s", providerID)
		}
		// 供应商选择只切换连接、密钥和模型；系统提示词和上下文策略继续沿用全局配置。
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

// ResolveFallbackModelConfig 把全局配置中的备用供应商选择解析成可直接调用的完整配置。
// 备用模型只继承当前请求的提示词、上下文和采样参数，不改变当前会话选中的主模型。
func (s *Store) ResolveFallbackModelConfig(base model.ModelConfig) (*model.ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	base = model.NormalizeModelConfig(base)
	providerID := modelprovider.NormalizeID(base.FallbackProviderID)
	modelName := strings.TrimSpace(base.FallbackModel)
	if providerID == "" && modelName == "" {
		return nil, nil
	}
	if providerID == "" {
		return nil, fmt.Errorf("fallback model provider is required")
	}
	providerCfg, ok, err := s.modelProviderConfigLocked(providerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("fallback model provider not found: %s", providerID)
	}
	if modelName == "" {
		modelName = providerCfg.Model
	}
	if modelName == "" {
		return nil, fmt.Errorf("fallback model is required")
	}

	fallback := base
	fallback.ProviderID = providerCfg.ProviderID
	fallback.BaseURL = providerCfg.BaseURL
	fallback.APIKey = providerCfg.APIKey
	fallback.Model = modelName
	fallback.Models = append([]string(nil), providerCfg.Models...)
	fallback.FallbackProviderID = ""
	fallback.FallbackModel = ""
	fallback = model.NormalizeModelConfig(fallback)
	if fallback.ProviderID == base.ProviderID && fallback.Model == base.Model {
		return nil, nil
	}
	return &fallback, nil
}

func applyProviderToConfigWith(reader sqlQueryer, cfg model.ModelConfig) (model.ModelConfig, error) {
	cfg = model.NormalizeModelConfig(cfg)
	providerCfg, ok, err := modelProviderConfigWith(reader, cfg.ProviderID)
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
