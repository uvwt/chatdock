package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/model"
)

type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"servers"`
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
	return "{\n  \"servers\": {}\n}\n"
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
	for serverName, server := range cfg.Servers {
		if err := validateToolExposure(server.ToolExposure, true); err != nil {
			return MCPConfig{}, fmt.Errorf("mcp server %s: %w", serverName, err)
		}
		for toolName, exposure := range server.ToolOverrides {
			if strings.TrimSpace(toolName) == "" {
				return MCPConfig{}, fmt.Errorf("mcp server %s: tool_overrides contains an empty tool name", serverName)
			}
			if err := validateToolExposure(exposure, false); err != nil {
				return MCPConfig{}, fmt.Errorf("mcp server %s tool %s: %w", serverName, toolName, err)
			}
		}
	}
	return cfg, nil
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
