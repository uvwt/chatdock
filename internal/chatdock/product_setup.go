package chatdock

import (
	"net/http"
	"strings"
	"time"
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
	defaultCfg := DefaultModelConfig()
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
		cfg.SystemPrompt = DefaultModelConfig().SystemPrompt
	}
	s.modelCfg = NormalizeModelConfig(cfg)
	err = s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
	s.mu.Unlock()
	if err != nil {
		return SetupStatus{}, err
	}
	return s.SetupStatus()
}

func (a *App) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.store.SetupStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}

func (a *App) handleSetupInit(w http.ResponseWriter, r *http.Request) {
	var input SetupInitRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, err := a.store.InitializeSetup(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, status)
}
