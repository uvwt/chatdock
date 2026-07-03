package store

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) SetupStatus() (SetupStatus, error) {
	s.mu.RLock()
	active := s.activePrompt
	cfg := s.modelCfg
	dataDir := s.dataDir
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return SetupStatus{}, err
	}
	defaultCfg := model.DefaultModelConfig()
	hasProvider := strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != ""
	hasKey := strings.TrimSpace(cfg.APIKey) != ""
	looksDefaultWithoutKey := !hasKey && cfg.BaseURL == defaultCfg.BaseURL && cfg.Model == defaultCfg.Model
	return SetupStatus{
		NeedsSetup:       !hasProvider || len(prompts) == 0 || looksDefaultWithoutKey,
		HasModelProvider: hasProvider,
		HasAPIKey:        hasKey,
		HasWorkspace:     len(prompts) > 0,
		ActiveWorkspace:  active,
		WorkspaceCount:   len(prompts),
		DataDir:          dataDir,
	}, nil
}

func (s *Store) InitializeSetup(input SetupInitRequest) (SetupStatus, error) {
	name := strings.TrimSpace(input.WorkspaceName)
	if name == "" {
		name = defaultPromptName
	}
	name, err := normalizePromptName(name)
	if err != nil {
		return SetupStatus{}, err
	}

	s.mu.Lock()
	if exists, err := s.promptExistsLocked(name); err != nil {
		s.mu.Unlock()
		return SetupStatus{}, err
	} else if !exists {
		if err := s.insertPromptLocked(name, time.Now()); err != nil {
			s.mu.Unlock()
			return SetupStatus{}, err
		}
	}
	if err := s.loadPromptLocked(name); err != nil {
		s.mu.Unlock()
		return SetupStatus{}, err
	}
	cfg := s.modelCfg
	cfg.BaseURL = strings.TrimSpace(input.BaseURL)
	cfg.APIKey = strings.TrimSpace(input.APIKey)
	cfg.Model = strings.TrimSpace(input.Model)
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = model.DefaultModelConfig().SystemPrompt
	}
	s.modelCfg = model.NormalizeModelConfig(cfg)
	err = s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
	s.mu.Unlock()
	if err != nil {
		return SetupStatus{}, err
	}
	return s.SetupStatus()
}
