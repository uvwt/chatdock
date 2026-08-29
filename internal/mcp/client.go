package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultMCPTimeout              = 90 * time.Second
	mcpInstructionDiscoveryTimeout = 8 * time.Second
	mcpInstructionFailureBackoff   = 30 * time.Second
	maxMCPResponseBodyBytes        = 16 << 20
	maxMCPAppHTMLBytes             = 4 << 20
	mcpAppExtension                = "io.modelcontextprotocol/ui"
	mcpAppMIMEType                 = "text/html;profile=mcp-app"
	chatDockMCPClientVersion       = "dev"
)

var ErrMCPAppToolForbidden = errors.New("mcp app tool forbidden")

type MCPClient struct {
	mu                sync.Mutex
	toolsCache        map[string]cachedMCPTools
	sessions          map[string]*mcpServerSession
	discoveryFailures map[string]cachedMCPDiscoveryFailure
	closed            bool
}

type mcpServerSession struct {
	cacheKey   string
	serverName string
	session    *mcpsdk.ClientSession
	info       MCPServerInfo
}

type cachedMCPTools struct {
	createdAt time.Time
	tools     []MCPTool
}

type cachedMCPDiscoveryFailure struct {
	until time.Time
	err   error
}

func NewMCPClient() *MCPClient {
	return &MCPClient{
		toolsCache:        map[string]cachedMCPTools{},
		sessions:          map[string]*mcpServerSession{},
		discoveryFailures: map[string]cachedMCPDiscoveryFailure{},
	}
}

func (c *MCPClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	sessions := make([]*mcpsdk.ClientSession, 0, len(c.sessions))
	for _, cached := range c.sessions {
		sessions = append(sessions, cached.session)
	}
	c.sessions = map[string]*mcpServerSession{}
	c.toolsCache = map[string]cachedMCPTools{}
	c.discoveryFailures = map[string]cachedMCPDiscoveryFailure{}
	c.mu.Unlock()

	var closeErr error
	for _, session := range sessions {
		closeErr = errors.Join(closeErr, session.Close())
	}
	return closeErr
}

func (c *MCPClient) ListTools(ctx context.Context, cfg MCPConfig) ([]MCPTool, error) {
	c.pruneSessionsForConfig(cfg)
	serverNames := enabledServerNames(cfg)
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

// ServerInfo 确保完成一次 MCP 协议协商，但不会主动加载 tools/list。
// 这让 on-demand server 的 instructions 也能在模型第一次决策前生效。
func (c *MCPClient) ServerInfo(ctx context.Context, cfg MCPConfig, serverName string) (MCPServerInfo, error) {
	server, ok := cfg.Servers[serverName]
	if !ok {
		return MCPServerInfo{}, fmt.Errorf("mcp server not found: %s", serverName)
	}
	if server.Disabled || strings.TrimSpace(server.URL) == "" {
		return MCPServerInfo{}, fmt.Errorf("mcp server unavailable: %s", serverName)
	}
	connected, err := c.ensureServerSession(ctx, serverName, server)
	if err != nil {
		return MCPServerInfo{}, err
	}
	return cloneServerInfo(connected.info), nil
}

func (c *MCPClient) ServerInstructions(ctx context.Context, cfg MCPConfig) ([]MCPServerInstruction, map[string]error) {
	c.pruneSessionsForConfig(cfg)
	serverNames := enabledServerNames(cfg)
	type discoveryResult struct {
		server string
		info   MCPServerInfo
		err    error
	}
	results := make(chan discoveryResult, len(serverNames))
	for _, serverName := range serverNames {
		serverName := serverName
		go func() {
			info, err := c.serverInfoForInstructions(ctx, cfg, serverName)
			results <- discoveryResult{server: serverName, info: info, err: err}
		}()
	}

	instructions := make([]MCPServerInstruction, 0, len(serverNames))
	failures := map[string]error{}
	for range serverNames {
		result := <-results
		if result.err != nil {
			failures[result.server] = result.err
			continue
		}
		if text := strings.TrimSpace(result.info.Instructions); text != "" {
			instructions = append(instructions, MCPServerInstruction{Server: result.server, Instructions: text})
		}
	}
	sort.Slice(instructions, func(i, j int) bool { return instructions[i].Server < instructions[j].Server })
	return instructions, failures
}

func (c *MCPClient) serverInfoForInstructions(ctx context.Context, cfg MCPConfig, serverName string) (MCPServerInfo, error) {
	server, ok := cfg.Servers[serverName]
	if !ok {
		return MCPServerInfo{}, fmt.Errorf("mcp server not found: %s", serverName)
	}
	cacheKey := serverCacheKey(serverName, server)
	now := time.Now()
	c.mu.Lock()
	if failure, ok := c.discoveryFailures[cacheKey]; ok {
		if now.Before(failure.until) {
			c.mu.Unlock()
			return MCPServerInfo{}, nil
		}
		delete(c.discoveryFailures, cacheKey)
	}
	c.mu.Unlock()

	discoveryCtx, cancel := context.WithTimeout(ctx, mcpInstructionDiscoveryTimeout)
	defer cancel()
	info, err := c.ServerInfo(discoveryCtx, cfg, serverName)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.discoveryFailures[cacheKey] = cachedMCPDiscoveryFailure{until: time.Now().Add(mcpInstructionFailureBackoff), err: err}
		return MCPServerInfo{}, err
	}
	delete(c.discoveryFailures, cacheKey)
	return info, nil
}

func (c *MCPClient) ListServerTools(ctx context.Context, cfg MCPConfig, serverName string) ([]MCPTool, error) {
	return c.listServerToolsForAudience(ctx, cfg, serverName, "model", true)
}

func (c *MCPClient) listServerToolsForAudience(ctx context.Context, cfg MCPConfig, serverName string, audience string, useCache bool) ([]MCPTool, error) {
	server, ok := cfg.Servers[serverName]
	if !ok {
		return nil, fmt.Errorf("mcp server not found: %s", serverName)
	}
	if server.Disabled || strings.TrimSpace(server.URL) == "" {
		return []MCPTool{}, nil
	}
	cacheKey := serverCacheKey(serverName, server)
	if useCache {
		if tools, ok := c.cachedTools(cacheKey, server); ok {
			return tools, nil
		}
	}

	connected, err := c.ensureServerSession(ctx, serverName, server)
	if err != nil {
		return nil, fmt.Errorf("%s connect failed: %w", serverName, err)
	}
	callCtx, cancel := serverCallContext(ctx, server)
	defer cancel()
	rawTools := make([]*mcpsdk.Tool, 0, 32)
	for tool, listErr := range connected.session.Tools(callCtx, nil) {
		if listErr != nil {
			c.invalidateSession(cacheKey, connected)
			return nil, fmt.Errorf("%s tools/list failed: %w", serverName, listErr)
		}
		rawTools = append(rawTools, tool)
	}

	tools := make([]MCPTool, 0, len(rawTools))
	toolAliases := make(map[string]string, len(rawTools))
	for _, rawTool := range rawTools {
		if rawTool == nil {
			continue
		}
		fullName := toolFullName(serverName, rawTool.Name)
		if !server.allowsTool(rawTool.Name, fullName) {
			continue
		}
		tool := normalizeSDKTool(serverName, fullName, rawTool)
		if !toolVisibleTo(tool.Meta, audience) {
			continue
		}
		if previous, exists := toolAliases[fullName]; exists {
			return nil, fmt.Errorf("%s tools %q and %q share exposed name %q", serverName, previous, rawTool.Name, fullName)
		}
		toolAliases[fullName] = rawTool.Name
		tools = append(tools, tool)
	}
	if useCache {
		c.storeCachedTools(cacheKey, tools)
	}
	return tools, nil
}

func (c *MCPClient) CallTool(ctx context.Context, cfg MCPConfig, fullName string, arguments map[string]any) (MCPToolResult, error) {
	return c.callTool(ctx, cfg, fullName, arguments, true)
}

func (c *MCPClient) CallToolAfterConfirmation(ctx context.Context, cfg MCPConfig, fullName string, arguments map[string]any) (MCPToolResult, error) {
	return c.callTool(ctx, cfg, fullName, arguments, false)
}

// CallAppTool executes a tools/call request originating from an MCP App.
// App calls are restricted to the selected server and to tools whose _meta.ui.visibility includes "app".
func (c *MCPClient) CallAppTool(ctx context.Context, cfg MCPConfig, serverName string, toolName string, arguments map[string]any) (MCPToolResult, error) {
	server, ok := cfg.Servers[serverName]
	if !ok || server.Disabled || strings.TrimSpace(server.URL) == "" {
		return MCPToolResult{}, fmt.Errorf("mcp app server unavailable: %s", serverName)
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return MCPToolResult{}, fmt.Errorf("mcp app tool name is empty")
	}
	tools, err := c.listServerToolsForAudience(ctx, cfg, serverName, "app", false)
	if err != nil {
		return MCPToolResult{}, err
	}
	var selected *MCPTool
	for i := range tools {
		if tools[i].Name == toolName {
			selected = &tools[i]
			break
		}
	}
	if selected == nil {
		return MCPToolResult{}, fmt.Errorf("%w: tool is not callable from app: %s", ErrMCPAppToolForbidden, toolName)
	}
	if server.requiresConfirmation(selected.Name, selected.FullName) {
		return MCPToolResult{}, fmt.Errorf("%w: tool requires manual confirmation: %s", ErrMCPAppToolForbidden, selected.FullName)
	}
	connected, err := c.ensureServerSession(ctx, serverName, server)
	if err != nil {
		return MCPToolResult{}, err
	}
	return c.invokeTool(ctx, serverName, server, connected, *selected, arguments)
}

func (c *MCPClient) ToolRequiresConfirmation(ctx context.Context, cfg MCPConfig, fullName string) (bool, error) {
	_, tool, server, _, err := c.resolveExposedTool(ctx, cfg, fullName)
	if err != nil {
		return false, err
	}
	return server.requiresConfirmation(tool.Name, tool.FullName), nil
}

func (c *MCPClient) callTool(ctx context.Context, cfg MCPConfig, fullName string, arguments map[string]any, enforceConfirmation bool) (MCPToolResult, error) {
	serverName, tool, server, connected, err := c.resolveExposedTool(ctx, cfg, fullName)
	if err != nil {
		return MCPToolResult{}, err
	}
	if !server.allowsTool(tool.Name, tool.FullName) {
		return MCPToolResult{}, fmt.Errorf("mcp tool is not allowed: %s", fullName)
	}
	if enforceConfirmation && server.requiresConfirmation(tool.Name, tool.FullName) {
		return MCPToolResult{}, fmt.Errorf("mcp tool requires manual confirmation: %s", fullName)
	}

	result, err := c.invokeTool(ctx, serverName, server, connected, tool, arguments)
	if err != nil {
		return MCPToolResult{}, err
	}

	resourceURI := toolUIResourceURI(result.Meta)
	if resourceURI == "" {
		resourceURI = toolUIResourceURI(tool.Meta)
	}
	if resourceURI != "" {
		app, appErr := c.readAppResource(ctx, serverName, server, connected, resourceURI)
		if appErr != nil {
			result.appError = appErr.Error()
		} else {
			result.appResource = app
		}
	}
	return result, nil
}

func (c *MCPClient) invokeTool(ctx context.Context, serverName string, server MCPServerConfig, connected *mcpServerSession, tool MCPTool, arguments map[string]any) (MCPToolResult, error) {
	callCtx, cancel := serverCallContext(ctx, server)
	defer cancel()
	sdkResult, err := connected.session.CallTool(callCtx, &mcpsdk.CallToolParams{Name: tool.Name, Arguments: arguments})
	if err != nil {
		c.invalidateSession(connected.cacheKey, connected)
		return MCPToolResult{}, fmt.Errorf("%s tools/call failed: %w", serverName, err)
	}
	result, err := normalizeSDKToolResult(sdkResult)
	if err != nil {
		return MCPToolResult{}, fmt.Errorf("normalize %s tool result: %w", tool.FullName, err)
	}
	return result, nil
}

func (c *MCPClient) resolveExposedTool(ctx context.Context, cfg MCPConfig, fullName string) (string, MCPTool, MCPServerConfig, *mcpServerSession, error) {
	serverName, _, server, err := ResolveToolServer(cfg, fullName)
	if err != nil {
		return "", MCPTool{}, MCPServerConfig{}, nil, err
	}
	if server.Disabled {
		return "", MCPTool{}, MCPServerConfig{}, nil, fmt.Errorf("mcp server disabled: %s", serverName)
	}
	tools, err := c.ListServerTools(ctx, cfg, serverName)
	if err != nil {
		return "", MCPTool{}, MCPServerConfig{}, nil, err
	}
	connected, err := c.ensureServerSession(ctx, serverName, server)
	if err != nil {
		return "", MCPTool{}, MCPServerConfig{}, nil, err
	}
	for _, tool := range tools {
		if tool.FullName == fullName {
			return serverName, tool, server, connected, nil
		}
	}
	return "", MCPTool{}, MCPServerConfig{}, nil, fmt.Errorf("mcp tool is not exposed: %s", fullName)
}

func (c *MCPClient) ensureServerSession(ctx context.Context, serverName string, server MCPServerConfig) (*mcpServerSession, error) {
	cacheKey := serverCacheKey(serverName, server)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("mcp client is closed")
	}
	if existing := c.sessions[cacheKey]; existing != nil {
		c.mu.Unlock()
		return existing, nil
	}
	c.mu.Unlock()

	caps := &mcpsdk.ClientCapabilities{}
	caps.AddExtension(mcpAppExtension, map[string]any{"mimeTypes": []string{mcpAppMIMEType}})
	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "chatdock",
		Title:   "ChatDock",
		Version: chatDockMCPVersion(),
	}, &mcpsdk.ClientOptions{Capabilities: caps})
	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             strings.TrimSpace(server.URL),
		HTTPClient:           serverHTTPClient(server),
		MaxRetries:           1,
		DisableStandaloneSSE: true,
	}
	connectCtx, cancel := serverCallContext(ctx, server)
	defer cancel()
	session, err := sdkClient.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, err
	}
	connected := &mcpServerSession{
		cacheKey:   cacheKey,
		serverName: serverName,
		session:    session,
		info:       normalizeSDKServerInfo(serverName, session.InitializeResult()),
	}

	var stale []*mcpsdk.ClientSession
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = session.Close()
		return nil, fmt.Errorf("mcp client is closed")
	}
	if existing := c.sessions[cacheKey]; existing != nil {
		c.mu.Unlock()
		_ = session.Close()
		return existing, nil
	}
	// 同一 server 配置变化后旧 session 不再复用，避免 bearer、URL 或协议状态串线。
	for key, existing := range c.sessions {
		if existing.serverName == serverName && key != cacheKey {
			stale = append(stale, existing.session)
			delete(c.sessions, key)
			delete(c.toolsCache, key)
		}
	}
	c.sessions[cacheKey] = connected
	c.mu.Unlock()
	for _, old := range stale {
		_ = old.Close()
	}
	return connected, nil
}

func chatDockMCPVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		version := strings.TrimSpace(info.Main.Version)
		if version != "" && version != "(devel)" {
			return strings.TrimPrefix(version, "v")
		}
	}
	return chatDockMCPClientVersion
}

func (c *MCPClient) pruneSessionsForConfig(cfg MCPConfig) {
	activeKeys := make(map[string]bool, len(cfg.Servers))
	for serverName, server := range cfg.Servers {
		if server.Disabled || strings.TrimSpace(server.URL) == "" {
			continue
		}
		activeKeys[serverCacheKey(serverName, server)] = true
	}

	var stale []*mcpsdk.ClientSession
	c.mu.Lock()
	for key, connected := range c.sessions {
		if activeKeys[key] {
			continue
		}
		stale = append(stale, connected.session)
		delete(c.sessions, key)
		delete(c.toolsCache, key)
	}
	for key := range c.discoveryFailures {
		if !activeKeys[key] {
			delete(c.discoveryFailures, key)
		}
	}
	c.mu.Unlock()
	for _, session := range stale {
		_ = session.Close()
	}
}

func (c *MCPClient) invalidateSession(cacheKey string, expected *mcpServerSession) {
	c.mu.Lock()
	if current := c.sessions[cacheKey]; current == expected {
		delete(c.sessions, cacheKey)
		delete(c.toolsCache, cacheKey)
	}
	c.mu.Unlock()
	if expected != nil && expected.session != nil {
		_ = expected.session.Close()
	}
}

func (c *MCPClient) readAppResource(ctx context.Context, serverName string, server MCPServerConfig, connected *mcpServerSession, resourceURI string) (*MCPAppResource, error) {
	parsed, err := url.Parse(strings.TrimSpace(resourceURI))
	if err != nil || parsed.Scheme != "ui" || parsed.Host == "" {
		return nil, fmt.Errorf("MCP App resource URI must use ui://: %s", resourceURI)
	}
	callCtx, cancel := serverCallContext(ctx, server)
	defer cancel()
	result, err := connected.session.ReadResource(callCtx, &mcpsdk.ReadResourceParams{URI: resourceURI})
	if err != nil {
		return nil, fmt.Errorf("%s resources/read %s failed: %w", serverName, resourceURI, err)
	}
	candidates := make([]*mcpsdk.ResourceContents, 0, len(result.Contents))
	var selected *mcpsdk.ResourceContents
	for _, content := range result.Contents {
		if content == nil || !isMCPAppMIMEType(content.MIMEType) {
			continue
		}
		candidates = append(candidates, content)
		if sameResourceURI(content.URI, resourceURI) {
			selected = content
			break
		}
	}
	if selected == nil && len(candidates) == 1 {
		// Some servers normalize the URI string in ResourceContents. With exactly one
		// MCP App candidate there is no ambiguity, so preserve interoperability.
		selected = candidates[0]
	}
	if selected == nil {
		return nil, fmt.Errorf("MCP App resource %s did not return unambiguous %s content", resourceURI, mcpAppMIMEType)
	}
	html := selected.Text
	if html == "" && len(selected.Blob) > 0 {
		if !utf8.Valid(selected.Blob) {
			return nil, fmt.Errorf("MCP App resource %s returned non-UTF-8 HTML blob", resourceURI)
		}
		html = string(selected.Blob)
	}
	if html == "" {
		return nil, fmt.Errorf("MCP App resource %s returned empty HTML", resourceURI)
	}
	if len(html) > maxMCPAppHTMLBytes {
		return nil, fmt.Errorf("MCP App resource %s exceeds %d bytes", resourceURI, maxMCPAppHTMLBytes)
	}
	return &MCPAppResource{
		Server:      serverName,
		ResourceURI: resourceURI,
		MIMEType:    selected.MIMEType,
		HTML:        html,
		Meta:        cloneJSONMap(map[string]any(selected.Meta)),
	}, nil
}

func sameResourceURI(left string, right string) bool {
	if strings.TrimSpace(left) == strings.TrimSpace(right) {
		return true
	}
	a, errA := url.Parse(strings.TrimSpace(left))
	b, errB := url.Parse(strings.TrimSpace(right))
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host) &&
		a.Path == b.Path && a.RawQuery == b.RawQuery && a.Fragment == b.Fragment
}

func normalizeSDKTool(serverName, fullName string, tool *mcpsdk.Tool) MCPTool {
	return MCPTool{
		Server:       serverName,
		Name:         tool.Name,
		FullName:     fullName,
		Title:        tool.Title,
		Description:  tool.Description,
		InputSchema:  normalizeJSONSchema(jsonMap(tool.InputSchema)),
		OutputSchema: jsonMap(tool.OutputSchema),
		Annotations:  jsonMap(tool.Annotations),
		Meta:         cloneJSONMap(map[string]any(tool.Meta)),
		Icons:        jsonMapSlice(tool.Icons),
	}
}

func normalizeSDKToolResult(result *mcpsdk.CallToolResult) (MCPToolResult, error) {
	if result == nil {
		return MCPToolResult{}, fmt.Errorf("empty MCP tool result")
	}
	content := make([]any, 0, len(result.Content))
	for _, item := range result.Content {
		if item == nil {
			continue
		}
		raw, err := item.MarshalJSON()
		if err != nil {
			return MCPToolResult{}, err
		}
		var normalized any
		if err := json.Unmarshal(raw, &normalized); err != nil {
			return MCPToolResult{}, err
		}
		content = append(content, normalized)
	}
	return MCPToolResult{
		Content:           content,
		StructuredContent: cloneJSONValue(result.StructuredContent),
		IsError:           result.IsError,
		Meta:              cloneJSONMap(map[string]any(result.Meta)),
	}, nil
}

func normalizeSDKServerInfo(serverName string, result *mcpsdk.InitializeResult) MCPServerInfo {
	info := MCPServerInfo{Server: serverName}
	if result == nil {
		return info
	}
	info.ProtocolVersion = result.ProtocolVersion
	info.Instructions = strings.TrimSpace(result.Instructions)
	info.Capabilities = jsonMap(result.Capabilities)
	info.Implementation = jsonMap(result.ServerInfo)
	return info
}

func cloneServerInfo(info MCPServerInfo) MCPServerInfo {
	info.Capabilities = cloneJSONMap(info.Capabilities)
	info.Implementation = cloneJSONMap(info.Implementation)
	return info
}

func toolUIResourceURI(meta map[string]any) string {
	ui, _ := meta["ui"].(map[string]any)
	if uri, _ := ui["resourceUri"].(string); strings.TrimSpace(uri) != "" {
		return strings.TrimSpace(uri)
	}
	// MCP Apps 2026-01-26 still defines the flat key as deprecated compatibility.
	legacy, _ := meta["ui/resourceUri"].(string)
	return strings.TrimSpace(legacy)
}

func toolVisibleTo(meta map[string]any, audience string) bool {
	ui, ok := meta["ui"].(map[string]any)
	if !ok {
		return true
	}
	raw, exists := ui["visibility"]
	if !exists {
		return true
	}
	switch visibility := raw.(type) {
	case []any:
		for _, item := range visibility {
			if value, ok := item.(string); ok && value == audience {
				return true
			}
		}
	case []string:
		for _, value := range visibility {
			if value == audience {
				return true
			}
		}
	}
	return false
}

func isMCPAppMIMEType(value string) bool {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(value))
	return err == nil && strings.EqualFold(mediaType, "text/html") && params["profile"] == "mcp-app"
}

func enabledServerNames(cfg MCPConfig) []string {
	names := make([]string, 0, len(cfg.Servers))
	for name, server := range cfg.Servers {
		if server.Disabled || strings.TrimSpace(server.URL) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func serverCallContext(ctx context.Context, server MCPServerConfig) (context.Context, context.CancelFunc) {
	timeout := defaultMCPTimeout
	if server.TimeoutMS > 0 {
		timeout = time.Duration(server.TimeoutMS) * time.Millisecond
	}
	return context.WithTimeout(ctx, timeout)
}

func serverHTTPClient(server MCPServerConfig) *http.Client {
	transport := http.RoundTripper(http.DefaultTransport)
	transport = &boundedMCPTransport{base: transport, maxBytes: maxMCPResponseBodyBytes}
	if token := server.bearerToken(); token != "" {
		transport = &bearerMCPTransport{base: transport, token: token, allowedOrigin: endpointOrigin(server.URL)}
	}
	return &http.Client{Transport: transport}
}

func endpointOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

type bearerMCPTransport struct {
	base          http.RoundTripper
	token         string
	allowedOrigin string
}

func (t *bearerMCPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || endpointOrigin(request.URL.String()) != t.allowedOrigin || t.allowedOrigin == "" {
		return t.base.RoundTrip(request)
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

type boundedMCPTransport struct {
	base     http.RoundTripper
	maxBytes int64
}

func (t *boundedMCPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if strings.EqualFold(mediaType, "text/event-stream") {
		response.Body = &boundedSSEReadCloser{ReadCloser: response.Body, maxEventBytes: t.maxBytes}
		return response, nil
	}
	if response.ContentLength > t.maxBytes {
		_ = response.Body.Close()
		return nil, fmt.Errorf("mcp response exceeds %d bytes", t.maxBytes)
	}
	response.Body = &boundedReadCloser{ReadCloser: response.Body, remaining: t.maxBytes}
	return response, nil
}

type boundedSSEReadCloser struct {
	io.ReadCloser
	maxEventBytes int64
	eventBytes    int64
	lineHasData   bool
}

func (r *boundedSSEReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	for _, b := range p[:n] {
		r.eventBytes++
		if b == '\n' {
			if !r.lineHasData {
				r.eventBytes = 0
			}
			r.lineHasData = false
		} else if b != '\r' {
			r.lineHasData = true
		}
		if r.eventBytes > r.maxEventBytes {
			return 0, fmt.Errorf("mcp SSE event exceeds %d bytes", r.maxEventBytes)
		}
	}
	return n, err
}

type boundedReadCloser struct {
	io.ReadCloser
	remaining int64
}

func (r *boundedReadCloser) Read(p []byte) (int, error) {
	if r.remaining < 0 {
		return 0, fmt.Errorf("mcp response exceeds %d bytes", maxMCPResponseBodyBytes)
	}
	if int64(len(p)) > r.remaining+1 {
		p = p[:r.remaining+1]
	}
	n, err := r.ReadCloser.Read(p)
	r.remaining -= int64(n)
	if r.remaining < 0 {
		return 0, fmt.Errorf("mcp response exceeds %d bytes", maxMCPResponseBodyBytes)
	}
	return n, err
}

func jsonMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func jsonMapSlice(value any) []map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
