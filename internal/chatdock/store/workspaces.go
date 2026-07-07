package store

import (
	"fmt"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

func (s *Store) ListWorkspaces() (WorkspaceResponse, error) {
	s.mu.RLock()
	active := s.activeWorkspace
	s.mu.RUnlock()

	prompts, err := s.listWorkspaceSummaries(active)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	items := make([]Workspace, 0, len(prompts))
	for _, prompt := range prompts {
		cfg, err := s.modelConfigForWorkspace(prompt.Name)
		if err != nil {
			return WorkspaceResponse{}, err
		}
		tasks, _ := s.scheduledTasksForWorkspace(prompt.Name)
		items = append(items, Workspace{
			ID:              prompt.Name,
			Name:            prompt.Name,
			Description:     workspaceDescription(prompt.Name),
			Icon:            "message-circle",
			ProviderID:      cfg.ProviderID,
			Model:           cfg.Model,
			Models:          append([]string(nil), cfg.Models...),
			SystemPrompt:    cfg.SystemPrompt,
			ContextLimit:    cfg.MaxContextMessages,
			Temperature:     cfg.Temperature,
			HideThinking:    cfg.HideThinking,
			EnableReasoning: cfg.EnableThinking,
			TaskCount:       len(tasks),
			SessionCount:    prompt.Count,
			Active:          prompt.Active,
			Archived:        false,
			CreatedAt:       prompt.CreatedAt,
			UpdatedAt:       prompt.UpdatedAt,
		})
	}
	return WorkspaceResponse{Active: active, Workspaces: items}, nil
}

func (s *Store) WorkspaceConfig(workspaceID string) (model.PublicModelConfig, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if exists, err := s.workspaceExistsLocked(workspaceID); err != nil {
		return model.PublicModelConfig{}, err
	} else if !exists {
		return model.PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	cfg, err := s.modelConfigForWorkspaceLocked(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	return model.ToPublicModelConfig(cfg), nil
}

func (s *Store) SaveWorkspaceConfig(workspaceID string, next model.ModelConfig) (model.PublicModelConfig, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.workspaceExistsLocked(workspaceID)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	if !exists {
		return model.PublicModelConfig{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	saved, err := s.saveModelConfigForWorkspaceLocked(workspaceID, next)
	if err != nil {
		return model.PublicModelConfig{}, err
	}
	// 工作空间配置可以在不切换当前会话空间的情况下保存；如果保存的是当前空间，同步内存态，避免后续聊天继续用旧配置。
	if workspaceID == s.activeWorkspace {
		s.modelCfg = saved
	}
	return model.ToPublicModelConfig(saved), nil
}

func (s *Store) PromptPreview(workspaceID string) (PromptPreviewResponse, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.activeWorkspace
	if err := s.loadWorkspaceLocked(workspaceID); err != nil {
		return PromptPreviewResponse{}, err
	}
	cfg := s.modelCfg
	content := llm.BuildSystemPrompt(cfg)
	if previous != workspaceID {
		if restoreErr := s.loadWorkspaceLocked(previous); restoreErr != nil && err == nil {
			err = restoreErr
		}
	}
	if err != nil {
		return PromptPreviewResponse{}, err
	}
	return PromptPreviewResponse{WorkspaceID: workspaceID, WorkspaceName: workspaceID, Content: content}, nil
}

func (s *Store) scheduledTasksForWorkspace(prompt string) ([]model.ScheduledTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks, err := loadScheduledTasksForWorkspaceLocked(s.db, prompt)
	if err != nil {
		return nil, err
	}
	return cloneScheduledTasks(tasks), nil
}

func workspaceDescription(name string) string {
	if name == defaultWorkspaceID {
		return "默认通用 AI 工作空间"
	}
	return "独立模型、提示词、工具和自动化配置"
}
