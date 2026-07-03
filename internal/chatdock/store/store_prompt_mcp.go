package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

func (s *Store) GetMCPConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, ok, err := s.getPromptRawLocked(s.activePrompt, "mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		return content, s.setPromptRawLocked(s.activePrompt, "mcp", content)
	}
	return content, nil
}

func (s *Store) GetEffectiveMCPConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, ok, err := s.getPromptRawLocked(s.activePrompt, "mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		if err := s.setPromptRawLocked(s.activePrompt, "mcp", content); err != nil {
			return "", err
		}
	}
	if mcpConfigHasServers(content) || s.activePrompt == defaultPromptName {
		return content, nil
	}

	fallback, ok, err := s.getPromptRawLocked(defaultPromptName, "mcp")
	if err != nil {
		return "", err
	}
	if ok && mcpConfigHasServers(fallback) {
		return fallback, nil
	}
	return content, nil
}

func mcpConfigHasServers(content string) bool {
	cfg, err := mcp.ParseMCPConfig(content)
	return err == nil && len(cfg.Servers) > 0
}

func (s *Store) SaveMCPConfig(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = DefaultMCPConfig()
	}
	var probe any
	if err := json.Unmarshal([]byte(content), &probe); err != nil {
		return "", fmt.Errorf("mcp config must be valid json: %w", err)
	}
	pretty, err := prettyJSON(content)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return pretty, s.setPromptRawLocked(s.activePrompt, "mcp", pretty)
}

func DefaultMCPConfig() string {
	return `{
  "servers": {}
}
`
}
