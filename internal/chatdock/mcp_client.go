package chatdock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type MCPClient struct {
	httpClient *http.Client
}

func NewMCPClient() *MCPClient {
	return &MCPClient{httpClient: &http.Client{Timeout: 90 * time.Second}}
}

type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Type string        `json:"type"`
	URL  string        `json:"url"`
	Auth MCPAuthConfig `json:"auth"`
}

type MCPAuthConfig struct {
	Type     string `json:"type"`
	Token    string `json:"token"`
	TokenEnv string `json:"token_env"`
}

type MCPTool struct {
	Server      string         `json:"server"`
	Name        string         `json:"name"`
	FullName    string         `json:"full_name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

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

func (c *MCPClient) ListTools(ctx context.Context, cfg MCPConfig) ([]MCPTool, error) {
	var all []MCPTool
	for serverName, server := range cfg.Servers {
		if strings.TrimSpace(server.URL) == "" {
			continue
		}
		var result struct {
			Tools []struct {
				Name        string         `json:"name"`
				Title       string         `json:"title"`
				Description string         `json:"description"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		}
		if err := c.call(ctx, server, "tools/list", map[string]any{}, &result); err != nil {
			return nil, fmt.Errorf("%s tools/list failed: %w", serverName, err)
		}
		for _, tool := range result.Tools {
			all = append(all, MCPTool{
				Server:      serverName,
				Name:        tool.Name,
				FullName:    toolFullName(serverName, tool.Name),
				Title:       tool.Title,
				Description: tool.Description,
				InputSchema: normalizeJSONSchema(tool.InputSchema),
			})
		}
	}
	return all, nil
}

func (c *MCPClient) CallTool(ctx context.Context, cfg MCPConfig, fullName string, arguments map[string]any) (any, error) {
	serverName, toolName := splitToolFullName(fullName)
	server, ok := cfg.Servers[serverName]
	if !ok {
		return nil, fmt.Errorf("mcp server not found: %s", serverName)
	}
	var result any
	err := c.call(ctx, server, "tools/call", map[string]any{"name": toolName, "arguments": arguments}, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *MCPClient) call(ctx context.Context, server MCPServerConfig, method string, params any, out any) error {
	endpoint := strings.TrimSpace(server.URL)
	if endpoint == "" {
		return fmt.Errorf("mcp server url is empty")
	}
	payload := map[string]any{"jsonrpc": "2.0", "id": time.Now().UnixNano(), "method": method, "params": params}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := server.bearerToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp http failed: %s: %s", resp.Status, string(respBody))
	}
	var rpc mcpJSONRPCResponse
	if err := json.Unmarshal(respBody, &rpc); err != nil {
		return err
	}
	if rpc.Error != nil {
		return fmt.Errorf("mcp error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if out == nil {
		return nil
	}
	if len(rpc.Result) == 0 {
		return nil
	}
	return json.Unmarshal(rpc.Result, out)
}

func (s MCPServerConfig) bearerToken() string {
	if !strings.EqualFold(strings.TrimSpace(s.Auth.Type), "bearer") {
		return ""
	}
	if token := strings.TrimSpace(s.Auth.Token); token != "" {
		return token
	}
	if env := strings.TrimSpace(s.Auth.TokenEnv); env != "" {
		return strings.TrimSpace(os.Getenv(env))
	}
	return ""
}

func toolFullName(serverName, toolName string) string {
	return safeToolName(serverName) + "__" + safeToolName(toolName)
}

func splitToolFullName(fullName string) (string, string) {
	parts := strings.SplitN(fullName, "__", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "agentdock", fullName
}

func safeToolName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "tool"
	}
	return b.String()
}

func normalizeJSONSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]any{}
	}
	return schema
}

func compactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(raw)
}
