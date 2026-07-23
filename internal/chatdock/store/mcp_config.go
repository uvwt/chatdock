package store

import (
	"fmt"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

func (s *Store) GetMCPConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok, err := s.getGlobalRawLocked("mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		return content, s.setGlobalRawLocked("mcp", content)
	}
	return content, nil
}

func (s *Store) GetEffectiveMCPConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok, err := s.getGlobalRawLocked("mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		if err := s.setGlobalRawLocked("mcp", content); err != nil {
			return "", err
		}
	}
	return content, nil
}

func (s *Store) SaveMCPConfig(content string) (string, error) {
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
	return pretty, s.setGlobalRawLocked("mcp", pretty)
}

func DefaultMCPConfig() string {
	return mcp.DefaultMCPConfig()
}
