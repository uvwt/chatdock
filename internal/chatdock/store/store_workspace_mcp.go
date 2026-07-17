package store

import (
	"encoding/json"
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
		return inheritMCPServersWithoutReplacingBuiltinTools(content, fallback)
	}
	return content, nil
}

func inheritMCPServersWithoutReplacingBuiltinTools(content, fallback string) (string, error) {
	var current map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &current); err != nil {
		return "", err
	}
	builtinTools, configured := current["builtin_tools"]
	if !configured {
		return fallback, nil
	}

	var inherited map[string]json.RawMessage
	if err := json.Unmarshal([]byte(fallback), &inherited); err != nil {
		return "", err
	}
	// MCP Server 可以继承默认工作空间，但内置工具策略属于当前工作空间，不能被继承配置覆盖。
	inherited["builtin_tools"] = builtinTools
	merged, err := json.MarshalIndent(inherited, "", "  ")
	if err != nil {
		return "", err
	}
	return string(merged) + "\n", nil
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
	return mcp.DefaultMCPConfig()
}
