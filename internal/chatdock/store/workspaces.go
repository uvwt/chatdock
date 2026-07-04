package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
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
			Models:            append([]string(nil), cfg.Models...),
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

func (s *Store) WorkspaceConfig(workspaceID string) (model.PublicModelConfig, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if exists, err := s.promptExistsLocked(workspaceID); err != nil {
		return model.PublicModelConfig{}, err
	} else if !exists {
		return model.PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	cfg, err := s.modelConfigForPromptLocked(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	return model.ToPublicModelConfig(cfg), nil
}

func (s *Store) SaveWorkspaceConfig(workspaceID string, next model.ModelConfig) (model.PublicModelConfig, error) {
	workspaceID, err := normalizePromptName(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	if !exists {
		return model.PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	current, err := s.modelConfigForPromptLocked(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
		next.APIKey = current.APIKey
	}
	if strings.TrimSpace(next.EmbeddingAPIKey) == "" || strings.TrimSpace(next.EmbeddingAPIKey) == "********" {
		next.EmbeddingAPIKey = current.EmbeddingAPIKey
	}
	if strings.TrimSpace(next.SystemPrompt) == "" {
		next.SystemPrompt = current.SystemPrompt
	}
	next = model.NormalizeModelConfig(next)
	// 工作空间配置可以在不切换当前会话空间的情况下保存；如果保存的是当前空间，同步内存态，避免后续聊天继续用旧配置。
	if workspaceID == s.activePrompt {
		s.modelCfg = next
	}
	if err := s.setPromptJSONLocked(workspaceID, "config", next); err != nil {
		return model.PublicModelConfig{}, err
	}
	return model.ToPublicModelConfig(next), nil
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
	content := llm.BuildSystemPrompt(cfg)
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

func (s *Store) skillsForPrompt(prompt string) ([]model.Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok, err := s.getPromptRawLocked(prompt, "skills")
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return []model.Skill{}, err
	}
	var skills []model.Skill
	if err := json.Unmarshal([]byte(raw), &skills); err != nil {
		return nil, err
	}
	sortSkills(skills)
	return cloneSkills(skills), nil
}

func (s *Store) scheduledTasksForPrompt(prompt string) ([]model.ScheduledTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok, err := s.getPromptRawLocked(prompt, "scheduled_tasks")
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return []model.ScheduledTask{}, err
	}
	var tasks []model.ScheduledTask
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
