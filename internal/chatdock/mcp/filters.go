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

func (s MCPServerConfig) ExposureForTool(toolName, fullName string) ToolExposure {
	// 未配置时默认按需加载，避免新接入的大型 MCP 一次性把全部 schema 塞给模型。
	return exposureForTool(s.ToolExposure, s.ToolOverrides, ToolExposureOnDemand, toolName, fullName)
}

func (c ToolExposureConfig) ExposureForTool(toolName, fullName string) ToolExposure {
	// 内置工具升级前始终直接加载，因此旧配置缺少 builtin_tools 时继续保持原行为。
	return exposureForTool(c.ToolExposure, c.ToolOverrides, ToolExposureDirect, toolName, fullName)
}

func exposureForTool(defaultExposure ToolExposure, overrides map[string]ToolExposure, fallback ToolExposure, toolName, fullName string) ToolExposure {
	if defaultExposure == "" {
		defaultExposure = fallback
	}
	for _, key := range []string{strings.TrimSpace(toolName), strings.TrimSpace(fullName)} {
		if key == "" {
			continue
		}
		exposure, ok := overrides[key]
		if !ok || exposure == "" || exposure == ToolExposureInherit {
			continue
		}
		return exposure
	}
	return defaultExposure
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
