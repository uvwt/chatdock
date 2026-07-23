package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/model"
)

type MCPConfig struct {
	BuiltinTools ToolExposureConfig         `json:"builtin_tools"`
	Servers      map[string]MCPServerConfig `json:"servers"`
}

type ToolExposure string

const (
	ToolExposureDirect   ToolExposure = "direct"
	ToolExposureOnDemand ToolExposure = "on_demand"
	ToolExposureInherit  ToolExposure = "inherit"
)

type MCPServerConfig struct {
	Type          string                  `json:"type"`
	URL           string                  `json:"url"`
	Description   string                  `json:"description"`
	Auth          MCPAuthConfig           `json:"auth"`
	Disabled      bool                    `json:"disabled"`
	AllowTools    []string                `json:"allow_tools"`
	DenyTools     []string                `json:"deny_tools"`
	ConfirmTools  []string                `json:"confirm_tools"`
	ToolExposure  ToolExposure            `json:"tool_exposure"`
	ToolOverrides map[string]ToolExposure `json:"tool_overrides"`
	TimeoutMS     int                     `json:"timeout_ms"`
	CacheTTLMS    int                     `json:"cache_ttl_ms"`
}

type ToolExposureConfig struct {
	ToolExposure  ToolExposure            `json:"tool_exposure"`
	ToolOverrides map[string]ToolExposure `json:"tool_overrides"`
}

type MCPAuthConfig struct {
	Type     string `json:"type"`
	Token    string `json:"token"`
	TokenEnv string `json:"token_env"`
}

type MCPTool = model.MCPTool

type MCPToolCallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type MCPToolCallResponse struct {
	Name   string `json:"name"`
	Result any    `json:"result"`
}

type mcpJSONRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      any              `json:"id"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *mcpJSONRPCError `json:"error,omitempty"`
}

type mcpJSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func DefaultMCPConfig() string {
	return "{\n  \"builtin_tools\": {\n    \"tool_exposure\": \"direct\"\n  },\n  \"servers\": {}\n}\n"
}

func ParseMCPConfig(content string) (MCPConfig, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = DefaultMCPConfig()
	}
	var cfg MCPConfig
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return MCPConfig{}, err
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]MCPServerConfig{}
	}
	if err := validateToolExposureConfig("builtin tools", cfg.BuiltinTools, true); err != nil {
		return MCPConfig{}, err
	}
	serverAliases := make(map[string]string, len(cfg.Servers))
	for serverName, server := range cfg.Servers {
		if strings.TrimSpace(serverName) == "" {
			return MCPConfig{}, fmt.Errorf("mcp server name cannot be empty")
		}
		if strings.EqualFold(strings.TrimSpace(serverName), "ChatDock") {
			return MCPConfig{}, fmt.Errorf("mcp server name %q is reserved for the built-in ChatDock resource", serverName)
		}
		alias := safeToolName(serverName)
		if previous, exists := serverAliases[alias]; exists {
			return MCPConfig{}, fmt.Errorf("mcp server names %q and %q share tool alias %q", previous, serverName, alias)
		}
		serverAliases[alias] = serverName
		if err := validateToolExposureConfig("mcp server "+serverName, ToolExposureConfig{
			ToolExposure:  server.ToolExposure,
			ToolOverrides: server.ToolOverrides,
		}, true); err != nil {
			return MCPConfig{}, err
		}
	}
	return cfg, nil
}

func validateToolExposureConfig(label string, config ToolExposureConfig, allowEmptyDefault bool) error {
	if err := validateToolExposure(config.ToolExposure, allowEmptyDefault); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	for toolName, exposure := range config.ToolOverrides {
		if strings.TrimSpace(toolName) == "" {
			return fmt.Errorf("%s: tool_overrides contains an empty tool name", label)
		}
		if err := validateToolExposure(exposure, false); err != nil {
			return fmt.Errorf("%s tool %s: %w", label, toolName, err)
		}
	}
	return nil
}

func validateToolExposure(exposure ToolExposure, serverDefault bool) error {
	switch exposure {
	case "", ToolExposureDirect, ToolExposureOnDemand:
		return nil
	case ToolExposureInherit:
		if !serverDefault {
			return nil
		}
	}
	return fmt.Errorf("invalid tool exposure %q; expected direct or on_demand", exposure)
}
