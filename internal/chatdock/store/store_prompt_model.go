package store

import (
	"strings"

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

	if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
		next.APIKey = s.modelCfg.APIKey
	}

	s.modelCfg = model.NormalizeModelConfig(next)
	return s.modelCfg, s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
}
