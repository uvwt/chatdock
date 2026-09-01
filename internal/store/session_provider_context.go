package store

import (
	"strings"

	"chatdock/internal/model"
)

// SessionProviderContext 返回当前会话下一次模型调用会使用的配置和历史消息。
// 这里只读取并组装持久化状态，不追加消息，也不改变会话所选模型。
func (s *Store) SessionProviderContext(sessionID string) (model.ModelConfig, []model.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok, err := s.sessionLocked(strings.TrimSpace(sessionID))
	if err != nil {
		return model.ModelConfig{}, nil, err
	}
	if !ok {
		return model.ModelConfig{}, nil, model.ErrSessionNotFound
	}
	if err := s.freezeSessionPromptsLocked(session); err != nil {
		return model.ModelConfig{}, nil, err
	}

	cfg, err := s.modelConfigLocked()
	if err != nil {
		return model.ModelConfig{}, nil, err
	}
	cfg, err = s.resolveChatModelConfigLocked(cfg, session.ProviderID, session.Model)
	if err != nil {
		return model.ModelConfig{}, nil, err
	}
	cfg.SystemPrompt = BuildFinalSystemPrompt(session.SystemPromptSnapshot, session.ProjectPromptSnapshot)

	return cfg, cloneMessages(session.Messages), nil
}
