package store

import (
	"fmt"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

func (s *Store) GetMCPConfig(workspaceID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return "", err
	}
	content, ok, err := s.getWorkspaceRawLocked(workspaceID, "mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		return content, s.setWorkspaceRawLocked(workspaceID, "mcp", content)
	}
	return content, nil
}

func (s *Store) GetEffectiveMCPConfig(workspaceID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return "", err
	}
	content, ok, err := s.getWorkspaceRawLocked(workspaceID, "mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		if err := s.setWorkspaceRawLocked(workspaceID, "mcp", content); err != nil {
			return "", err
		}
	}
	if mcpConfigHasServers(content) || workspaceID == defaultWorkspaceID {
		return content, nil
	}
	fallback, ok, err := s.getWorkspaceRawLocked(defaultWorkspaceID, "mcp")
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

func (s *Store) SaveMCPConfig(workspaceID string, content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = DefaultMCPConfig()
	}
	if _, err := mcp.ParseMCPConfig(content); err != nil {
		return "", fmt.Errorf("invalid mcp config: %w", err)
	}
	pretty, err := prettyJSON(content)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err = s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return "", err
	}
	return pretty, s.setWorkspaceRawLocked(workspaceID, "mcp", pretty)
}

func DefaultMCPConfig() string {
	return `{
  "servers": {}
}
`
}
