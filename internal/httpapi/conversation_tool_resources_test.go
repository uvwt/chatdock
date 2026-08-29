package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type testMCPResource struct {
	server    *httptest.Server
	listCalls atomic.Int32
}

func newTestMCPResource(t *testing.T, tools []map[string]any, status int) *testMCPResource {
	t.Helper()
	resource := &testMCPResource{}
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "chatdock-test-resource", Version: "1"}, nil)
	for _, rawTool := range tools {
		raw, err := json.Marshal(rawTool)
		if err != nil {
			t.Fatal(err)
		}
		var tool mcpsdk.Tool
		if err := json.Unmarshal(raw, &tool); err != nil {
			t.Fatal(err)
		}
		sdkServer.AddTool(&tool, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{}}, nil
		})
	}
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	resource.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			handler.ServeHTTP(w, r)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var request struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Errorf("decode MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Method == "tools/list" {
			resource.listCalls.Add(1)
			if status != http.StatusOK {
				http.Error(w, "resource unavailable", status)
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(resource.server.Close)
	return resource
}

func newResourceTestApp(t *testing.T, cfg mcp.MCPConfig) *Server {
	t.Helper()
	app, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveMCPConfig(string(raw)); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestOnDemandResourceDefersToolsListUntilSelected(t *testing.T) {
	calendar := newTestMCPResource(t, []map[string]any{{
		"name":        "events_create",
		"title":       "创建日历事件",
		"description": "创建新的日历安排",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}}},
	}}, http.StatusOK)
	mail := newTestMCPResource(t, []map[string]any{{
		"name":        "messages_search",
		"title":       "搜索邮件",
		"description": "按关键词搜索邮件",
		"inputSchema": map[string]any{"type": "object"},
	}}, http.StatusOK)

	app := newResourceTestApp(t, mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {URL: calendar.server.URL, Description: "查询和管理日历安排", ToolExposure: mcp.ToolExposureOnDemand},
		"mail":     {URL: mail.server.URL, Description: "搜索和处理电子邮件", ToolExposure: mcp.ToolExposureOnDemand},
	}})
	toolSet, _, err := app.loadConversationTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := calendar.listCalls.Load(); got != 0 {
		t.Fatalf("on-demand calendar resource should not list tools during setup, got %d calls", got)
	}
	if got := mail.listCalls.Load(); got != 0 {
		t.Fatalf("on-demand mail resource should not list tools during setup, got %d calls", got)
	}
	if toolSet.resources["calendar"].info.Loaded || toolSet.resources["mail"].info.Loaded {
		t.Fatalf("on-demand resources should remain unloaded: %#v", toolSet.resourceIndex())
	}

	result, err := app.discoverConversationTools(context.Background(), toolSet, map[string]any{"resources": []string{"calendar"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := calendar.listCalls.Load(); got != 1 {
		t.Fatalf("selected calendar resource should list tools once, got %d", got)
	}
	if got := mail.listCalls.Load(); got != 0 {
		t.Fatalf("unselected mail resource must remain lazy, got %d calls", got)
	}
	loaded, _ := result["loaded_tools"].([]string)
	if len(loaded) != 1 || loaded[0] != "calendar__events_create" {
		t.Fatalf("single-resource bulk load should expose its real tool, got %#v", result)
	}
	if !toolSet.visibleNames["calendar__events_create"] {
		t.Fatal("loaded real tool should be directly callable in the next model request")
	}

	if _, err := app.discoverConversationTools(context.Background(), toolSet, map[string]any{"resources": []string{"calendar"}}); err != nil {
		t.Fatal(err)
	}
	if got := calendar.listCalls.Load(); got != 1 {
		t.Fatalf("loaded resource should reuse its definitions, got %d tools/list calls", got)
	}
}

func TestResourceIndexIsIncludedInDiscoveryTool(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"DockMini": {URL: "http://example.invalid", Description: "Mac mini 本机开发、文件、命令和 Git 能力", ToolExposure: mcp.ToolExposureOnDemand},
	}}
	set := newConversationToolSet(builtinChatDockTools(), cfg)
	var discovery mcp.MCPTool
	for _, tool := range set.tools() {
		if tool.FullName == builtinToolSearchTools {
			discovery = tool
			break
		}
	}
	if discovery.FullName == "" {
		t.Fatal("on-demand resource should expose the discovery entrypoint")
	}
	if !strings.Contains(discovery.Description, "DockMini") || !strings.Contains(discovery.Description, "Mac mini 本机开发") {
		t.Fatalf("resource index should be present in discovery metadata, got %q", discovery.Description)
	}
	properties, _ := discovery.InputSchema["properties"].(map[string]any)
	if _, ok := properties["resources"]; !ok {
		t.Fatalf("discovery schema should support resource selection: %#v", discovery.InputSchema)
	}
	if _, required := discovery.InputSchema["required"]; required {
		t.Fatalf("query must be optional for single-resource bulk loading: %#v", discovery.InputSchema)
	}
}

func TestDirectResourceFailureDoesNotDisableOtherResources(t *testing.T) {
	broken := newTestMCPResource(t, nil, http.StatusBadGateway)
	healthy := newTestMCPResource(t, []map[string]any{{
		"name":        "ping",
		"title":       "连通性检查",
		"description": "检查资源是否可用",
		"inputSchema": map[string]any{"type": "object"},
	}}, http.StatusOK)
	app := newResourceTestApp(t, mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"broken":  {URL: broken.server.URL, ToolExposure: mcp.ToolExposureDirect},
		"healthy": {URL: healthy.server.URL, ToolExposure: mcp.ToolExposureDirect},
	}})

	var setupErrors []map[string]any
	toolSet, _, err := app.loadConversationTools(context.Background(), func(event string, value any) error {
		if event == "tool_setup_error" {
			payload, _ := value.(map[string]any)
			setupErrors = append(setupErrors, payload)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !toolSet.visibleNames["healthy__ping"] {
		t.Fatalf("healthy resource tool should remain visible: %#v", toolSet.tools())
	}
	if resource := toolSet.resources["broken"]; resource.info.Status != "error" || resource.info.Loaded {
		t.Fatalf("broken resource should be isolated as an unloadable error: %#v", resource.info)
	}
	if resource := toolSet.resources["healthy"]; resource.info.Status != "ready" || !resource.info.Loaded {
		t.Fatalf("healthy resource should finish loading: %#v", resource.info)
	}
	if len(setupErrors) != 1 || setupErrors[0]["resource"] != "broken" {
		t.Fatalf("expected one resource-scoped setup error, got %#v", setupErrors)
	}
}

func TestMultiResourceBulkLoadHonorsSchemaBudget(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"first":  {ToolExposure: mcp.ToolExposureOnDemand},
		"second": {ToolExposure: mcp.ToolExposureOnDemand},
	}}
	tools := make([]mcp.MCPTool, 0, maxBulkResourceToolDefinitions+1)
	for i := 0; i < maxBulkResourceToolDefinitions+1; i++ {
		server := "first"
		if i%2 == 1 {
			server = "second"
		}
		name := fmt.Sprintf("tool_%02d", i)
		tools = append(tools, mcp.MCPTool{Server: server, Name: name, FullName: server + "__" + name, InputSchema: map[string]any{"type": "object"}})
	}
	set := newConversationToolSet(tools, cfg)
	result, err := (&Server{}).discoverConversationTools(context.Background(), set, map[string]any{"resources": []string{"first", "second"}})
	if err != nil {
		t.Fatal(err)
	}
	if exceeded, _ := result["budget_exceeded"].(bool); !exceeded {
		t.Fatalf("multi-resource load above budget should not expose all schemas: %#v", result)
	}
	loaded, _ := result["loaded_tools"].([]string)
	if len(loaded) != 0 {
		t.Fatalf("budget rejection must not partially expose tools, got %#v", loaded)
	}
}

func TestGlobalSearchMatchesChineseResourceAndToolText(t *testing.T) {
	calendar := newTestMCPResource(t, []map[string]any{{
		"name":        "events_create",
		"title":       "创建日历事件",
		"description": "创建新的日历安排",
		"inputSchema": map[string]any{"type": "object"},
	}}, http.StatusOK)
	mail := newTestMCPResource(t, []map[string]any{{
		"name":        "messages_search",
		"title":       "搜索邮件",
		"description": "按关键词搜索电子邮件",
		"inputSchema": map[string]any{"type": "object"},
	}}, http.StatusOK)
	app := newResourceTestApp(t, mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {URL: calendar.server.URL, Description: "查询和管理日历安排", ToolExposure: mcp.ToolExposureOnDemand},
		"mail":     {URL: mail.server.URL, Description: "搜索和处理电子邮件", ToolExposure: mcp.ToolExposureOnDemand},
	}})
	toolSet, _, err := app.loadConversationTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := app.discoverConversationTools(context.Background(), toolSet, map[string]any{"query": "创建日历安排"})
	if err != nil {
		t.Fatal(err)
	}
	if got := calendar.listCalls.Load(); got != 1 {
		t.Fatalf("calendar resource should be selected by Chinese resource text, got %d calls", got)
	}
	if got := mail.listCalls.Load(); got != 0 {
		t.Fatalf("unrelated mail resource should remain lazy, got %d calls", got)
	}
	loaded, _ := result["loaded_tools"].([]string)
	if len(loaded) != 1 || loaded[0] != "calendar__events_create" {
		t.Fatalf("Chinese keyword fragments should match the target tool, got %#v", result)
	}
}

func TestToolDiscoveryRejectsOversizedInput(t *testing.T) {
	set := newConversationToolSet(builtinChatDockTools(), mcp.MCPConfig{})
	app := &Server{}

	tooLong := strings.Repeat("查", maxToolDiscoveryQueryRunes+1)
	if _, err := app.discoverConversationTools(context.Background(), set, map[string]any{"query": tooLong}); err == nil || !strings.Contains(err.Error(), "query exceeds") {
		t.Fatalf("oversized query should be rejected, got %v", err)
	}

	resources := make([]string, maxToolDiscoveryResourceCount+1)
	for i := range resources {
		resources[i] = fmt.Sprintf("resource-%d", i)
	}
	if _, err := app.discoverConversationTools(context.Background(), set, map[string]any{"query": "test", "resources": resources}); err == nil || !strings.Contains(err.Error(), "resources exceeds") {
		t.Fatalf("too many resources should be rejected before lookup, got %v", err)
	}
}

func TestResourceDescriptionAndErrorAreCompacted(t *testing.T) {
	resource := newMCPToolResource("demo", mcp.MCPServerConfig{URL: "http://example.invalid", Description: "  第一行\n\n第二行  "})
	if resource.info.Description != "第一行 第二行" {
		t.Fatalf("resource description should be compacted, got %q", resource.info.Description)
	}
	resource.info.LastError = strings.Repeat("错误", 200)
	payload := resourceIndexPayload([]toolResource{resource.info})
	errorText, _ := payload[0]["error"].(string)
	if len([]rune(errorText)) > 181 || !strings.HasSuffix(errorText, "…") {
		t.Fatalf("resource error should be bounded, got %d runes: %q", len([]rune(errorText)), errorText)
	}
}
