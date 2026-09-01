package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

func addWorkingSetUserTurns(t *testing.T, app *Server, sessionID string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, _, _, err := app.store.PrepareChat(model.ChatRequest{SessionID: sessionID, Message: fmt.Sprintf("第 %d 轮", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConversationWorkingSetDoesNotExposeSchemasAcrossTurns(t *testing.T) {
	calendar := newTestMCPResource(t, []map[string]any{{
		"name":        "events_update",
		"title":       "更新日历事件",
		"description": "修改已有日历安排",
		"inputSchema": map[string]any{"type": "object"},
	}}, http.StatusOK)
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {URL: calendar.server.URL, Description: "日历", ToolExposure: mcp.ToolExposureOnDemand},
	}}
	app := newResourceTestApp(t, cfg)
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	addWorkingSetUserTurns(t, app, session.ID, 1)
	entry := storepkg.SessionToolWorkingSetEntry{ToolName: "calendar__events_update", ResourceID: "calendar"}
	if err := app.store.RecordSessionToolDiscovery(session.ID, 1, []storepkg.SessionToolWorkingSetEntry{entry}); err != nil {
		t.Fatal(err)
	}

	set, _, err := app.loadConversationTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if set.exposure.Has(entry.ToolName) || set.discovered.Has(entry.ToolName) {
		t.Fatal("working-set history must not expose a real schema in a later turn")
	}
	if _, ok := set.visibleTool(entry.ToolName); ok {
		t.Fatal("working-set history must not make a real tool callable before search")
	}
	if got := calendar.listCalls.Load(); got != 0 {
		t.Fatalf("working-set history must not reload MCP catalogs, got %d calls", got)
	}
	if len(set.tools()) < 2 {
		t.Fatalf("on-demand tools should expose the two fixed proxy schemas, got %#v", set.tools())
	}
}

func TestConversationToolDiscoveryKeepsTopLevelToolsStable(t *testing.T) {
	calendar := newTestMCPResource(t, []map[string]any{{
		"name":        "events_create",
		"title":       "创建日历事件",
		"description": "创建新的日历安排",
		"inputSchema": map[string]any{"type": "object"},
	}}, http.StatusOK)
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {URL: calendar.server.URL, Description: "日历", ToolExposure: mcp.ToolExposureOnDemand},
	}}
	app := newResourceTestApp(t, cfg)
	set, _, err := app.loadConversationTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	before := set.tools()
	if _, err := app.discoverConversationTools(context.Background(), set, map[string]any{"resources": []string{"calendar"}}); err != nil {
		t.Fatal(err)
	}
	after := set.tools()
	if len(after) != len(before) {
		t.Fatalf("search must not change top-level tool count: before=%d after=%d", len(before), len(after))
	}
	for _, tool := range after {
		if tool.FullName == "calendar__events_create" {
			t.Fatal("real discovered schema must remain in the conversation tail, not top-level tools")
		}
	}
	if _, ok := set.visibleTool("calendar__events_create"); !ok {
		t.Fatal("the loaded real schema should remain callable through chatdock_tool_call")
	}
}

func TestFailedConversationToolCallDoesNotBecomeStrongSticky(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"demo": {URL: "http://example.invalid", ToolExposure: mcp.ToolExposureOnDemand},
	}}
	app := newResourceTestApp(t, cfg)
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	addWorkingSetUserTurns(t, app, session.ID, 1)
	tool := mcp.MCPTool{Server: "demo", Name: "update", FullName: "demo__update", InputSchema: map[string]any{"type": "object"}}
	set := newConversationToolSet([]mcp.MCPTool{tool}, cfg)
	set.workingSetSessionID = session.ID
	set.workingSetTurn = 1
	set.expose([]mcp.MCPTool{tool})

	_, err = app.callVisibleConversationTool(context.Background(), set, func(string, map[string]any) (any, error) {
		return nil, errors.New("remote call failed")
	}, tool.FullName, map[string]any{})
	if err == nil {
		t.Fatal("expected tool call failure")
	}
	entries, err := app.store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed call must not become strong sticky: %#v", entries)
	}

	if _, err := app.callVisibleConversationTool(context.Background(), set, func(string, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	}, tool.FullName, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	entries, err = app.store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].LastCalledTurn != 1 {
		t.Fatalf("successful call should be recorded for compatibility: %#v", entries)
	}
}
