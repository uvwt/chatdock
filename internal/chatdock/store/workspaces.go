package store

import (
	"fmt"
	"strings"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

func (s *Store) ListWorkspaces() (WorkspaceResponse, error) {
	s.mu.RLock()
	active := s.workspaceCacheID
	s.mu.RUnlock()

	workspaceSummaries, err := s.listWorkspaceSummaries(active)
	if err != nil {
		return WorkspaceResponse{}, err
	}
	items := make([]Workspace, 0, len(workspaceSummaries))
	for _, workspace := range workspaceSummaries {
		cfg, err := s.modelConfigForWorkspace(workspace.Name)
		if err != nil {
			return WorkspaceResponse{}, err
		}
		tasks, _ := s.scheduledTasksForWorkspace(workspace.Name)
		ready := workspaceReadinessFromConfig(workspace.Name, cfg)
		items = append(items, Workspace{
			ID:              workspace.Name,
			Name:            workspace.Name,
			Description:     workspaceDescription(workspace.Name),
			Icon:            "message-circle",
			ProviderID:      cfg.ProviderID,
			Model:           cfg.Model,
			Models:          append([]string(nil), cfg.Models...),
			SystemPrompt:    cfg.SystemPrompt,
			ContextLimit:    cfg.MaxContextMessages,
			Temperature:     cfg.Temperature,
			HideThinking:    cfg.HideThinking,
			EnableReasoning: cfg.EnableThinking,
			Ready:           ready.Ready,
			ReadinessReason: ready.Reason,
			TaskCount:       len(tasks),
			SessionCount:    workspace.Count,
			Active:          workspace.Active,
			Archived:        false,
			CreatedAt:       workspace.CreatedAt,
			UpdatedAt:       workspace.UpdatedAt,
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
	if workspaceID == s.workspaceCacheID {
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

	previous := s.workspaceCacheID
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

func (s *Store) WorkspaceReadiness(workspaceID string) (WorkspaceReadiness, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return WorkspaceReadiness{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if exists, err := s.workspaceExistsLocked(workspaceID); err != nil {
		return WorkspaceReadiness{}, err
	} else if !exists {
		return WorkspaceReadiness{}, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	cfg, err := s.modelConfigForWorkspaceLocked(workspaceID)
	if err != nil {
		return WorkspaceReadiness{}, err
	}
	return workspaceReadinessFromConfig(workspaceID, cfg), nil
}

func workspaceReadinessFromConfig(workspaceID string, cfg model.ModelConfig) WorkspaceReadiness {
	providerID := strings.TrimSpace(cfg.ProviderID)
	modelName := strings.TrimSpace(cfg.Model)
	hasProvider := strings.TrimSpace(cfg.BaseURL) != "" && modelName != ""
	hasKey := strings.TrimSpace(cfg.APIKey) != ""
	reason := ""
	if !hasProvider {
		reason = "模型供应商或模型未配置"
	} else if !hasKey {
		reason = "API Key 未配置"
	}
	return WorkspaceReadiness{WorkspaceID: workspaceID, Ready: hasProvider && hasKey, HasModelProvider: hasProvider, HasAPIKey: hasKey, Model: modelName, ProviderID: providerID, Reason: reason}
}

func workspaceDescription(name string) string {
	if name == defaultWorkspaceID {
		return "默认通用 AI 工作空间"
	}
	return "独立模型、提示词、工具和自动化配置"
}
