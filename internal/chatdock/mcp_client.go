package chatdock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type MCPClient struct {
	httpClient *http.Client
	mu         sync.Mutex
	toolsCache map[string]cachedMCPTools
}

func NewMCPClient() *MCPClient {
	return &MCPClient{httpClient: &http.Client{Timeout: 90 * time.Second}, toolsCache: map[string]cachedMCPTools{}}
}

type cachedMCPTools struct {
	createdAt time.Time
	tools     []MCPTool
}

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
	serverNames := make([]string, 0, len(cfg.Servers))
	for serverName := range cfg.Servers {
		serverNames = append(serverNames, serverName)
	}
	sort.Strings(serverNames)

	var all []MCPTool
	for _, serverName := range serverNames {
		tools, err := c.ListServerTools(ctx, cfg, serverName)
		if err != nil {
			return nil, err
		}
		all = append(all, tools...)
	}
	return all, nil
}

func (c *MCPClient) ListServerTools(ctx context.Context, cfg MCPConfig, serverName string) ([]MCPTool, error) {
	server, ok := cfg.Servers[serverName]
	if !ok {
		return nil, fmt.Errorf("mcp server not found: %s", serverName)
	}
	if server.Disabled || strings.TrimSpace(server.URL) == "" {
		return []MCPTool{}, nil
	}
	cacheKey := serverCacheKey(serverName, server)
	if tools, ok := c.cachedTools(cacheKey, server); ok {
		return tools, nil
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

	tools := make([]MCPTool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		fullName := toolFullName(serverName, tool.Name)
		if !server.allowsTool(tool.Name, fullName) {
			continue
		}
		tools = append(tools, MCPTool{
			Server:      serverName,
			Name:        tool.Name,
			FullName:    fullName,
			Title:       tool.Title,
			Description: tool.Description,
			InputSchema: normalizeJSONSchema(tool.InputSchema),
		})
	}
	c.storeCachedTools(cacheKey, tools)
	return tools, nil
}

func (c *MCPClient) CallTool(ctx context.Context, cfg MCPConfig, fullName string, arguments map[string]any) (any, error) {
	serverName, toolName := splitToolFullName(fullName)
	server, ok := cfg.Servers[serverName]
	if !ok {
		return nil, fmt.Errorf("mcp server not found: %s", serverName)
	}
	if server.Disabled {
		return nil, fmt.Errorf("mcp server disabled: %s", serverName)
	}
	if !server.allowsTool(toolName, fullName) {
		return nil, fmt.Errorf("mcp tool is not allowed: %s", fullName)
	}
	if server.requiresConfirmation(toolName, fullName) {
		return nil, fmt.Errorf("mcp tool requires manual confirmation: %s", fullName)
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
	if server.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(server.TimeoutMS)*time.Millisecond)
		defer cancel()
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
	if out == nil || len(rpc.Result) == 0 {
		return nil
	}
	return json.Unmarshal(rpc.Result, out)
}

func (c *MCPClient) cachedTools(key string, server MCPServerConfig) ([]MCPTool, bool) {
	ttl := server.cacheTTL()
	if ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.toolsCache[key]
	if !ok || time.Since(item.createdAt) > ttl {
		return nil, false
	}
	return cloneTools(item.tools), true
}

func (c *MCPClient) storeCachedTools(key string, tools []MCPTool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.toolsCache[key] = cachedMCPTools{createdAt: time.Now(), tools: cloneTools(tools)}
}

func (s MCPServerConfig) cacheTTL() time.Duration {
	if s.CacheTTLMS < 0 {
		return 0
	}
	if s.CacheTTLMS > 0 {
		return time.Duration(s.CacheTTLMS) * time.Millisecond
	}
	return 30 * time.Second
}

func serverCacheKey(serverName string, server MCPServerConfig) string {
	raw, _ := json.Marshal(server)
	sum := sha256.Sum256(append([]byte(serverName+":"), raw...))
	return hex.EncodeToString(sum[:])
}

func cloneTools(tools []MCPTool) []MCPTool {
	out := make([]MCPTool, len(tools))
	copy(out, tools)
	return out
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

func (s MCPServerConfig) allowsTool(toolName, fullName string) bool {
	if matchesAny(toolName, fullName, s.DenyTools) {
		return false
	}
	if len(s.AllowTools) == 0 {
		return true
	}
	return matchesAny(toolName, fullName, s.AllowTools)
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
	out := make(map[string]any, len(schema)+2)
	for k, v := range schema {
		out[k] = v
	}
	if _, ok := out["type"]; !ok {
		out["type"] = "object"
	}
	if _, ok := out["properties"]; !ok {
		out["properties"] = map[string]any{}
	}
	return out
}

func compactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(raw)
}
