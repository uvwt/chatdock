package store

import "strings"

func BuildFinalSystemPrompt(globalPrompt string, projectPrompt string) string {
	globalPrompt = strings.TrimSpace(globalPrompt)
	projectPrompt = strings.TrimSpace(projectPrompt)
	switch {
	case globalPrompt == "":
		return projectPrompt
	case projectPrompt == "":
		return globalPrompt
	default:
		return globalPrompt + "\n\n" + projectPrompt
	}
}

func (s *Store) projectPromptForSessionLocked(projectID string) (string, bool, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", false, nil
	}
	project, ok, err := projectByIDWith(s.db, projectID)
	if err != nil || !ok {
		return "", ok, err
	}
	return project.Prompt, true, nil
}
