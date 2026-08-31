package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestParseMCPConfigRejectsAmbiguousServerToolAliases(t *testing.T) {
	for _, content := range []string{
		`{"servers":{"":{"url":"https://example.test"}}}`,
		`{"servers":{"calendar prod":{},"calendar_prod":{}}}`,
	} {
		if _, err := ParseMCPConfig(content); err == nil {
			t.Fatalf("expected invalid server name aliases to fail: %s", content)
		}
	}
}

func TestSDKClientNegotiatesLatestProtocolAndPreservesMetadataAndApps(t *testing.T) {
	const (
		defaultResourceURI = "ui://demo/default"
		resourceURI        = "ui://demo/result"
	)
	readOnly := true
	sdkServer := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "demo-server", Version: "1.0.0"},
		&mcpsdk.ServerOptions{Instructions: "Call context before mutating data."},
	)
	sdkServer.AddTool(&mcpsdk.Tool{
		Name:         "inspect",
		Title:        "Inspect",
		Description:  "Inspect the demo resource.",
		InputSchema:  map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
		OutputSchema: map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}}},
		Annotations:  &mcpsdk.ToolAnnotations{ReadOnlyHint: readOnly, Title: "Read demo"},
		Meta:         mcpsdk.Meta{"ui": map[string]any{"resourceUri": defaultResourceURI}, "demo": "tool-meta"},
		Icons:        []mcpsdk.Icon{{Source: "data:image/svg+xml;base64,AA==", MIMEType: "image/svg+xml"}},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{
			Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: "done"}},
			StructuredContent: map[string]any{"answer": "ok"},
			Meta:              mcpsdk.Meta{"result": "meta", "ui": map[string]any{"resourceUri": resourceURI}},
		}, nil
	})
	sdkServer.AddResource(&mcpsdk.Resource{URI: resourceURI, Name: "demo-ui", MIMEType: mcpAppMIMEType}, func(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{
			URI: resourceURI, MIMEType: mcpAppMIMEType, Text: "<!doctype html><script>parent.postMessage({jsonrpc:'2.0',method:'ui/notifications/size-changed',params:{height:42}},'*')</script>",
			Meta: mcpsdk.Meta{"ui": map[string]any{"csp": map[string]any{"connectDomains": []string{}}}},
		}}}, nil
	})
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	var mu sync.Mutex
	var discover map[string]any
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accept := r.Header.Get("Accept"); !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
			t.Errorf("Accept = %q, want JSON and event stream", accept)
		}
		if r.Method == http.MethodPost {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(raw)))
			var message map[string]any
			if json.Unmarshal(raw, &message) == nil && message["method"] == "server/discover" {
				mu.Lock()
				discover = message
				mu.Unlock()
			}
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: httpServer.URL}}}
	info, err := client.ServerInfo(context.Background(), cfg, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if info.ProtocolVersion != "2026-07-28" || info.Instructions != "Call context before mutating data." {
		t.Fatalf("unexpected server info: %#v", info)
	}
	if info.Implementation["name"] != "demo-server" {
		t.Fatalf("server implementation = %#v", info.Implementation)
	}

	mu.Lock()
	gotDiscover := discover
	mu.Unlock()
	params, _ := gotDiscover["params"].(map[string]any)
	meta, _ := params["_meta"].(map[string]any)
	caps, _ := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any)
	extensions, _ := caps["extensions"].(map[string]any)
	appExtension, ok := extensions[mcpAppExtension].(map[string]any)
	if !ok {
		t.Fatalf("discover client capabilities missing %q: %#v", mcpAppExtension, caps)
	}
	mimeTypes, _ := appExtension["mimeTypes"].([]any)
	if len(mimeTypes) != 1 || mimeTypes[0] != mcpAppMIMEType {
		t.Fatalf("MCP Apps capability mimeTypes = %#v, want [%q]", mimeTypes, mcpAppMIMEType)
	}

	tools, err := client.ListServerTools(context.Background(), cfg, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %#v", tools)
	}
	tool := tools[0]
	if tool.OutputSchema["type"] != "object" || tool.Annotations["readOnlyHint"] != true || tool.Meta["demo"] != "tool-meta" || len(tool.Icons) != 1 {
		t.Fatalf("tool metadata was not preserved: %#v", tool)
	}

	result, err := client.CallTool(context.Background(), cfg, tool.FullName, map[string]any{"query": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || result.StructuredContent.(map[string]any)["answer"] != "ok" || result.Meta["result"] != "meta" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
	app := result.AppResource()
	if app == nil || app.ResourceURI != resourceURI || !strings.Contains(app.HTML, "size-changed") {
		t.Fatalf("MCP App resource = %#v, app error = %q", app, result.AppError())
	}
}

func TestSDKClientListsAllPaginatedTools(t *testing.T) {
	sdkServer := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "paged", Version: "1"},
		&mcpsdk.ServerOptions{PageSize: 1},
	)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		name := name
		sdkServer.AddTool(&mcpsdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{}}, nil
		})
	}
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()

	client := NewMCPClient()
	defer client.Close()
	tools, err := client.ListServerTools(context.Background(), MCPConfig{Servers: map[string]MCPServerConfig{"paged": {URL: server.URL}}}, "paged")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("paginated tools = %#v", tools)
	}
	got := []string{tools[0].Name, tools[1].Name, tools[2].Name}
	sort.Strings(got)
	if strings.Join(got, ",") != "alpha,beta,gamma" {
		t.Fatalf("paginated tool names = %#v", got)
	}
}

func TestToolListChangedInvalidatesChatDockToolsCache(t *testing.T) {
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "changing-tools", Version: "1"}, nil)
	addTool := func(name string) {
		sdkServer.AddTool(&mcpsdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{}}, nil
		})
	}
	addTool("alpha")
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true}))
	defer server.Close()

	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL, CacheTTLMS: 60_000}}}
	tools, err := client.ListServerTools(context.Background(), cfg, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "alpha" {
		t.Fatalf("initial tools = %#v", tools)
	}
	if _, ok := client.CachedServerTools(cfg, "demo"); !ok {
		t.Fatal("expected ChatDock normalized tools cache after initial list")
	}

	addTool("beta")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := client.CachedServerTools(cfg, "demo"); !ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := client.CachedServerTools(cfg, "demo"); ok {
		t.Fatal("notifications/tools/list_changed did not invalidate ChatDock tools cache")
	}

	tools, err = client.ListServerTools(context.Background(), cfg, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools after list_changed = %#v", tools)
	}
	names := []string{tools[0].Name, tools[1].Name}
	sort.Strings(names)
	if strings.Join(names, ",") != "alpha,beta" {
		t.Fatalf("tools after list_changed = %#v", tools)
	}
}

func TestMCPAppInvalidMIMEFallsBackToNormalToolResult(t *testing.T) {
	const resourceURI = "ui://demo/not-an-app"
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "bad-ui", Version: "1"}, nil)
	sdkServer.AddTool(&mcpsdk.Tool{
		Name: "inspect", InputSchema: map[string]any{"type": "object"},
		Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": resourceURI}},
	}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "functional result"}}}, nil
	})
	sdkServer.AddResource(&mcpsdk.Resource{URI: resourceURI, Name: "plain-html", MIMEType: "text/html"}, func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{URI: resourceURI, MIMEType: "text/html", Text: "<html>not an MCP App</html>"}}}, nil
	})
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()

	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL}}}
	result, err := client.CallTool(context.Background(), cfg, "demo__inspect", map[string]any{})
	if err != nil {
		t.Fatalf("functional tool result should survive App presentation failure: %v", err)
	}
	if result.AppResource() != nil || !strings.Contains(result.AppError(), mcpAppMIMEType) {
		t.Fatalf("invalid MIME app=%#v error=%q", result.AppResource(), result.AppError())
	}
	if len(result.Content) != 1 {
		t.Fatalf("functional content lost: %#v", result.Content)
	}
}

func TestSDKClientFallsBackToLegacyInitialize(t *testing.T) {
	var mu sync.Mutex
	methods := []string{}
	const sessionID = "legacy-session"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var request struct {
			JSONRPC string         `json:"jsonrpc"`
			ID      any            `json:"id"`
			Method  string         `json:"method"`
			Params  map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		methods = append(methods, request.Method)
		mu.Unlock()
		switch request.Method {
		case "server/discover":
			writeRPC(t, w, request.ID, nil, map[string]any{"code": -32601, "message": "Method not found"})
		case "initialize":
			w.Header().Set("Mcp-Session-Id", sessionID)
			writeRPC(t, w, request.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "legacy", "version": "1"},
				"instructions":    "Legacy guidance.",
			}, nil)
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") != sessionID {
				t.Errorf("initialized missing session id: %q", r.Header.Get("Mcp-Session-Id"))
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if r.Header.Get("Mcp-Session-Id") != sessionID {
				t.Errorf("tools/list missing session id: %q", r.Header.Get("Mcp-Session-Id"))
			}
			writeRPC(t, w, request.ID, map[string]any{"tools": []map[string]any{{"name": "legacy_tool", "inputSchema": map[string]any{"type": "object"}}}}, nil)
		default:
			t.Errorf("unexpected method %q", request.Method)
			writeRPC(t, w, request.ID, nil, map[string]any{"code": -32601, "message": "Method not found"})
		}
	}))
	defer server.Close()

	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"legacy": {URL: server.URL}}}
	info, err := client.ServerInfo(context.Background(), cfg, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if info.ProtocolVersion != "2025-11-25" || info.Instructions != "Legacy guidance." {
		t.Fatalf("legacy info = %#v", info)
	}
	tools, err := client.ListServerTools(context.Background(), cfg, "legacy")
	if err != nil || len(tools) != 1 {
		t.Fatalf("legacy tools = %#v, err = %v", tools, err)
	}
	mu.Lock()
	joined := strings.Join(methods, ",")
	mu.Unlock()
	if !strings.Contains(joined, "server/discover,initialize,notifications/initialized,tools/list") {
		t.Fatalf("legacy handshake methods = %s", joined)
	}
}

func TestCallToolResolvesSanitizedServerAliasToOriginalConfigName(t *testing.T) {
	var calledTool string
	originalToolName := "events create/中文"
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "calendar", Version: "1"}, nil)
	sdkServer.AddTool(&mcpsdk.Tool{Name: originalToolName, InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		calledTool = req.Params.Name
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}, StructuredContent: map[string]any{"ok": true}}, nil
	})
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()

	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"calendar prod": {URL: server.URL}}}
	client := NewMCPClient()
	defer client.Close()
	fullName := ToolFullName("calendar prod", originalToolName)
	result, err := client.CallTool(context.Background(), cfg, fullName, map[string]any{"title": "周会"})
	if err != nil {
		t.Fatal(err)
	}
	if calledTool != originalToolName {
		t.Fatalf("called tool = %q", calledTool)
	}
	if !strings.Contains(CompactJSON(result), `"ok":true`) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestListServerToolsRejectsSanitizedToolAliasCollision(t *testing.T) {
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "demo", Version: "1"}, nil)
	for _, name := range []string{"event create", "event_create"} {
		name := name
		sdkServer.AddTool(&mcpsdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{}}, nil
		})
	}
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()

	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL}}}
	_, err := client.ListServerTools(context.Background(), cfg, "demo")
	if err == nil || !strings.Contains(err.Error(), "share exposed name") {
		t.Fatalf("tool alias collision error = %v", err)
	}
}

func TestToolRequiresConfirmationUsesOriginalToolName(t *testing.T) {
	originalToolName := "events delete/中文"
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "demo", Version: "1"}, nil)
	sdkServer.AddTool(&mcpsdk.Tool{Name: originalToolName, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{}}, nil
	})
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()

	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"calendar prod": {URL: server.URL, ConfirmTools: []string{originalToolName}}}}
	required, err := client.ToolRequiresConfirmation(context.Background(), cfg, ToolFullName("calendar prod", originalToolName))
	if err != nil || !required {
		t.Fatalf("confirmation required=%v error=%v", required, err)
	}
}

func TestBoundedMCPTransportLimitsEachSSEEventNotWholeStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: 1234567890\n\ndata: abcdefghij\n\ndata: klmnopqrst\n\n")
	}))
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&boundedMCPTransport{base: http.DefaultTransport, maxBytes: 24}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("multi-event SSE stream should not use cumulative byte limit: %v", err)
	}
}

func TestBoundedMCPTransportRejectsOversizedSingleSSEEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+strings.Repeat("x", 40)+"\n\n")
	}))
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&boundedMCPTransport{base: http.DefaultTransport, maxBytes: 24}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err == nil || !strings.Contains(err.Error(), "SSE event exceeds") {
		t.Fatalf("oversized SSE event error = %v", err)
	}
}

func TestMCPTransportRejectsDeclaredResponseOverflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "20000000")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL}}}
	_, err := client.ServerInfo(context.Background(), cfg, "demo")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestServerInstructionsPrunesSessionsRemovedFromConfig(t *testing.T) {
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "prune", Version: "1"}, &mcpsdk.ServerOptions{Instructions: "hello"})
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()
	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL}}}
	if _, err := client.ServerInfo(context.Background(), cfg, "demo"); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	before := len(client.sessions)
	client.mu.Unlock()
	if before != 1 {
		t.Fatalf("cached sessions before prune = %d", before)
	}
	client.ServerInstructions(context.Background(), MCPConfig{Servers: map[string]MCPServerConfig{}})
	client.mu.Lock()
	after := len(client.sessions)
	client.mu.Unlock()
	if after != 0 {
		t.Fatalf("cached sessions after prune = %d", after)
	}
}

func TestServerInstructionsDiscoversServersConcurrentlyAndBacksOffFailures(t *testing.T) {
	var reached atomic.Int32
	barrier := make(chan struct{})
	var closeOnce sync.Once
	newBlockedServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				ID any `json:"id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			if reached.Add(1) == 2 {
				closeOnce.Do(func() { close(barrier) })
			}
			select {
			case <-barrier:
				writeRPC(t, w, request.ID, nil, map[string]any{"code": -32601, "message": "blocked test server"})
			case <-r.Context().Done():
				return
			}
		}))
	}
	first := newBlockedServer()
	defer first.Close()
	second := newBlockedServer()
	defer second.Close()

	cfg := MCPConfig{Servers: map[string]MCPServerConfig{
		"a": {URL: first.URL, TimeoutMS: 1000},
		"b": {URL: second.URL, TimeoutMS: 1000},
	}}
	client := NewMCPClient()
	defer client.Close()

	started := time.Now()
	_, failures := client.ServerInstructions(context.Background(), cfg)
	elapsed := time.Since(started)
	if len(failures) != 2 {
		t.Fatalf("failures = %#v, want both servers", failures)
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("instruction discovery took %s; servers were not discovered concurrently", elapsed)
	}
	firstCount := reached.Load()
	_, failures = client.ServerInstructions(context.Background(), cfg)
	if len(failures) != 0 {
		t.Fatalf("backoff should suppress repeated failure noise: %#v", failures)
	}
	if reached.Load() != firstCount {
		t.Fatalf("failed discovery was retried during backoff: before=%d after=%d", firstCount, reached.Load())
	}
}

func TestBearerTransportDoesNotLeakAcrossRedirectOrigin(t *testing.T) {
	var redirectedAuth string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	var initialAuth string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initialAuth = r.Header.Get("Authorization")
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	client := serverHTTPClient(MCPServerConfig{URL: origin.URL, Auth: MCPAuthConfig{Type: "bearer", Token: "secret-token"}})
	request, err := http.NewRequest(http.MethodPost, origin.URL, strings.NewReader(`{"jsonrpc":"2.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if initialAuth != "Bearer secret-token" {
		t.Fatalf("initial Authorization = %q", initialAuth)
	}
	if redirectedAuth != "" {
		t.Fatalf("Bearer token leaked to redirect origin: %q", redirectedAuth)
	}
}

func TestToolUIResourceURISupportsDeprecatedFlatKey(t *testing.T) {
	if got := toolUIResourceURI(map[string]any{"ui/resourceUri": "ui://legacy/app"}); got != "ui://legacy/app" {
		t.Fatalf("legacy resource URI = %q", got)
	}
	if got := toolUIResourceURI(map[string]any{"ui": map[string]any{"resourceUri": "ui://modern/app"}, "ui/resourceUri": "ui://legacy/app"}); got != "ui://modern/app" {
		t.Fatalf("modern resource URI should win, got %q", got)
	}
}

func writeRPC(t *testing.T, w http.ResponseWriter, id any, result any, rpcError any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	payload := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcError != nil {
		payload["error"] = rpcError
	} else {
		payload["result"] = result
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Error(err)
	}
}

func TestMCPAppToolVisibilitySeparatesModelAndAppAudiences(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "visibility", Version: "1"}, nil)
	add := func(name string, visibility []any) {
		meta := mcpsdk.Meta{}
		if visibility != nil {
			meta["ui"] = map[string]any{"visibility": visibility}
		}
		sdkServer.AddTool(&mcpsdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}, Meta: meta}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			mu.Lock()
			calls = append(calls, name)
			mu.Unlock()
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: name}}}, nil
		})
	}
	add("default_tool", nil)
	add("model_only", []any{"model"})
	add("app_only", []any{"app"})

	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL}}}
	client := NewMCPClient()
	defer client.Close()

	modelTools, err := client.ListServerTools(context.Background(), cfg, "demo")
	if err != nil {
		t.Fatal(err)
	}
	modelNames := map[string]bool{}
	for _, tool := range modelTools {
		modelNames[tool.Name] = true
	}
	if !modelNames["default_tool"] || !modelNames["model_only"] || modelNames["app_only"] {
		t.Fatalf("model-visible tools = %#v", modelNames)
	}
	if _, err := client.CallTool(context.Background(), cfg, "demo__app_only", nil); err == nil || !strings.Contains(err.Error(), "not exposed") {
		t.Fatalf("model unexpectedly called app-only tool: %v", err)
	}
	if _, err := client.CallAppTool(context.Background(), cfg, "demo", "model_only", nil); err == nil || !strings.Contains(err.Error(), "not callable from app") {
		t.Fatalf("app unexpectedly called model-only tool: %v", err)
	}
	if _, err := client.CallAppTool(context.Background(), cfg, "demo", "app_only", nil); err != nil {
		t.Fatalf("app-only tool call failed: %v", err)
	}
	if _, err := client.CallAppTool(context.Background(), cfg, "demo", "default_tool", nil); err != nil {
		t.Fatalf("default visibility should be callable from app: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(calls, ",") != "app_only,default_tool" {
		t.Fatalf("upstream calls = %#v", calls)
	}
}

func TestMCPAppResourceAcceptsUTF8BlobContent(t *testing.T) {
	const resourceURI = "ui://demo/blob"
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "blob-ui", Version: "1"}, nil)
	sdkServer.AddTool(&mcpsdk.Tool{
		Name: "inspect", InputSchema: map[string]any{"type": "object"},
		Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": resourceURI}},
	}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}}, nil
	})
	sdkServer.AddResource(&mcpsdk.Resource{URI: resourceURI, Name: "blob-ui", MIMEType: mcpAppMIMEType}, func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{URI: resourceURI, MIMEType: mcpAppMIMEType, Blob: []byte("<!doctype html><p>blob app</p>")}}}, nil
	})
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer server.Close()
	client := NewMCPClient()
	defer client.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL}}}
	result, err := client.CallTool(context.Background(), cfg, "demo__inspect", nil)
	if err != nil {
		t.Fatal(err)
	}
	app := result.AppResource()
	if app == nil || !strings.Contains(app.HTML, "blob app") {
		t.Fatalf("blob app resource = %#v, error=%q", app, result.AppError())
	}
}
