package mcp

import (
	"encoding/json"
	"strings"

	"chatdock/internal/chatdock/model"
)

type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Type         string        `json:"type"`
	URL          string        `json:"url"`
	Auth         MCPAuthConfig `json:"auth"`
	Disabled     bool          `json:"disabled"`
	AllowTools   []string      `json:"allow_tools"`
	DenyTools    []string      `json:"deny_tools"`
	ConfirmTools []string      `json:"confirm_tools"`
	TimeoutMS    int           `json:"timeout_ms"`
	CacheTTLMS   int           `json:"cache_ttl_ms"`
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
	return cfg, nil
}
