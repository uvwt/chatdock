package store

import (
	"strings"

	"chatdock/internal/model"
)

func (s *Store) SessionSystemPrompt(sessionID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok, err := s.sessionLocked(strings.TrimSpace(sessionID))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", model.ErrSessionNotFound
	}
	cfg, err := s.modelConfigLocked()
	if err != nil {
		return "", err
	}
	prompt := cfg.SystemPrompt
	if projectPrompt, found, err := s.projectPromptForSessionLocked(session.ProjectID); err != nil {
		return "", err
	} else if found {
		prompt = BuildFinalSystemPrompt(prompt, projectPrompt)
	}
	return prompt, nil
}
