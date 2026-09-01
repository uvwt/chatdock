package store

import "chatdock/internal/model"

func (s *Store) freezeSessionPromptsLocked(session *model.Session) error {
	if session == nil {
		return nil
	}
	cfg, err := s.modelConfigLocked()
	if err != nil {
		return err
	}
	if !session.SystemPromptFrozen {
		session.SystemPromptSnapshot = cfg.SystemPrompt
		session.SystemPromptFrozen = true
	}
	if session.ProjectID == "" || session.ProjectPromptFrozen {
		return nil
	}
	prompt, ok, err := s.projectPromptForSessionLocked(session.ProjectID)
	if err != nil {
		return err
	}
	if ok {
		session.ProjectPromptSnapshot = prompt
	}
	session.ProjectPromptFrozen = true
	return nil
}
