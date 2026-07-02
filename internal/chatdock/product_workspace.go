package chatdock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *Store) ListWorkspaces() (WorkspaceResponse, error) {
	s.mu.RLock()
	active := s.activePrompt
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	items := make([]Workspace, 0, len(prompts))
	for _, prompt := range prompts {
		cfg, err := s.modelConfigForPrompt(prompt.Name)
		if err != nil {
			return WorkspaceResponse{}, err
		}
		skills, _ := s.skillsForPrompt(prompt.Name)
		tasks, _ := s.scheduledTasksForPrompt(prompt.Name)
		enabledSkills := 0
		for _, skill := range skills {
			if skill.Enabled {
				enabledSkills++
			}
		}
		items = append(items, Workspace{
			ID:                prompt.Name,
			Name:              prompt.Name,
			Description:       workspaceDescription(prompt.Name),
			Icon:              "message-circle",
			ProviderID:        cfg.ProviderID,
			Model:             cfg.Model,
			SystemPrompt:      cfg.SystemPrompt,
			ContextLimit:      cfg.MaxContextMessages,
			Temperature:       cfg.Temperature,
			HideThinking:      cfg.HideThinking,
			EnableReasoning:   cfg.EnableThinking,
			SkillCount:        len(skills),
			EnabledSkillCount: enabledSkills,
			TaskCount:         len(tasks),
			SessionCount:      prompt.Count,
			Active:            prompt.Active,
			Archived:          false,
			CreatedAt:         prompt.CreatedAt,
			UpdatedAt:         prompt.UpdatedAt,
		})
	}
	return WorkspaceResponse{Active: active, Workspaces: items}, nil
}

func (s *Store) WorkspaceConfig(workspaceID string) (PublicModelConfig, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if exists, err := s.promptExistsLocked(workspaceID); err != nil {
		return PublicModelConfig{}, err
	} else if !exists {
		return PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	cfg, err := s.modelConfigForPromptLocked(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	return ToPublicModelConfig(cfg), nil
}

func (s *Store) SaveWorkspaceConfig(workspaceID string, next ModelConfig) (PublicModelConfig, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	if !exists {
		return PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	current, err := s.modelConfigForPromptLocked(workspaceID)
	if err != nil {
		return PublicModelConfig{}, err
	}
	if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
		next.APIKey = current.APIKey
	}
	if strings.TrimSpace(next.SystemPrompt) == "" {
		next.SystemPrompt = current.SystemPrompt
	}
	next = NormalizeModelConfig(next)
	// 工作空间配置可以在不切换当前会话空间的情况下保存；如果保存的是当前空间，同步内存态，避免后续聊天继续用旧配置。
	if workspaceID == s.activePrompt {
		s.modelCfg = next
	}
	if err := s.setPromptJSONLocked(workspaceID, "config", next); err != nil {
		return PublicModelConfig{}, err
	}
	return ToPublicModelConfig(next), nil
}

func (s *Store) PromptPreview(workspaceID string) (PromptPreviewResponse, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.activePrompt
	if err := s.loadPromptLocked(workspaceID); err != nil {
		return PromptPreviewResponse{}, err
	}
	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	cfg.Skills = skills
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	content := buildSystemPrompt(cfg)
	if previous != workspaceID {
		if restoreErr := s.loadPromptLocked(previous); restoreErr != nil && err == nil {
			err = restoreErr
		}
	}
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	return PromptPreviewResponse{WorkspaceID: workspaceID, WorkspaceName: workspaceID, SkillNames: names, Content: content}, nil
}

func (s *Store) skillsForPrompt(prompt string) ([]Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok, err := s.getPromptRawLocked(prompt, "skills")
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return []Skill{}, err
	}
	var skills []Skill
	if err := json.Unmarshal([]byte(raw), &skills); err != nil {
		return nil, err
	}
	sortSkills(skills)
	return cloneSkills(skills), nil
}

func (s *Store) scheduledTasksForPrompt(prompt string) ([]ScheduledTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok, err := s.getPromptRawLocked(prompt, "scheduled_tasks")
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return []ScheduledTask{}, err
	}
	var tasks []ScheduledTask
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		return nil, err
	}
	sortScheduledTasks(tasks)
	return cloneScheduledTasks(tasks), nil
}

func workspaceDescription(name string) string {
	if name == defaultPromptName {
		return "默认通用 AI 工作空间"
	}
	return "独立模型、提示词、技能、工具和自动化配置"
}

func (a *App) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var input CreatePromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.store.CreatePrompt(input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleWorkspaceRoute(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workspaces/"), "/")
	parts := strings.Split(requestPath, "/")
	if len(parts) != 1 && len(parts) != 2 {
		writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
		return
	}

	workspaceID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if _, err := a.store.DeletePrompt(SelectPromptRequest{Name: workspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.ListWorkspaces()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
		return
	}
	// 配置中心使用 /api/workspaces 作为产品化资源入口；旧 /api/prompts 继续兼容侧栏提示词空间。
	if parts[1] == "select" && r.Method == http.MethodPost {
		if _, err := a.store.SelectPrompt(SelectPromptRequest{Name: workspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.ListWorkspaces()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if parts[1] == "config" && r.Method == http.MethodGet {
		cfg, err := a.store.WorkspaceConfig(workspaceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, cfg)
		return
	}
	if parts[1] == "config" && r.Method == http.MethodPost {
		var input ModelConfig
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg, err := a.store.SaveWorkspaceConfig(workspaceID, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, cfg)
		return
	}
	if len(parts) == 2 && parts[1] == "prompt-preview" && r.Method == http.MethodGet {
		preview, err := a.store.PromptPreview(workspaceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, preview)
		return
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
}
