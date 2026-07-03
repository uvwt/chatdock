package mcp

import (
	"os"
	"strings"
)

func (s MCPServerConfig) BearerToken() string {
	return s.bearerToken()
}

func (s MCPServerConfig) bearerToken() string {
	if !strings.EqualFold(strings.TrimSpace(s.Auth.Type), "bearer") {
		return ""
	}
	if token := normalizeBearerToken(s.Auth.Token); token != "" {
		return token
	}
	if env := strings.TrimSpace(s.Auth.TokenEnv); env != "" {
		return normalizeBearerToken(os.Getenv(env))
	}
	return ""
}

func normalizeBearerToken(value string) string {
	token := strings.TrimSpace(value)
	if len(token) >= len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
		return strings.TrimSpace(token[len("Bearer "):])
	}
	return token
}

func (s MCPServerConfig) allowsTool(toolName, fullName string) bool {
	if matchesAny(toolName, fullName, s.DenyTools) {
		return false
	}
	if len(s.AllowTools) == 0 {
		return true
	}
	return matchesAny(toolName, fullName, s.AllowTools)
}

func (s MCPServerConfig) RequiresConfirmation(toolName, fullName string) bool {
	return s.requiresConfirmation(toolName, fullName)
}

func (s MCPServerConfig) requiresConfirmation(toolName, fullName string) bool {
	return matchesAny(toolName, fullName, s.ConfirmTools)
}

func matchesAny(toolName, fullName string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchToolPattern(toolName, pattern) || matchToolPattern(fullName, pattern) {
			return true
		}
	}
	return false
}

func matchToolPattern(value, pattern string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "*" || value == pattern {
		return true
	}
	if strings.HasSuffix(pattern, "*") && strings.HasPrefix(value, strings.TrimSuffix(pattern, "*")) {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(value, strings.TrimPrefix(pattern, "*")) {
		return true
	}
	return false
}
