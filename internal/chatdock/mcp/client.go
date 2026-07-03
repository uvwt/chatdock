package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	return c.callTool(ctx, cfg, fullName, arguments, true)
}

func (c *MCPClient) CallToolAfterConfirmation(ctx context.Context, cfg MCPConfig, fullName string, arguments map[string]any) (any, error) {
	return c.callTool(ctx, cfg, fullName, arguments, false)
}

func (c *MCPClient) callTool(ctx context.Context, cfg MCPConfig, fullName string, arguments map[string]any, enforceConfirmation bool) (any, error) {
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
	if enforceConfirmation && server.requiresConfirmation(toolName, fullName) {
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
