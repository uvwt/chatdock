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

func TestConversationToolWorkingSetRestoresDiscoveredToolOnNextTurn(t *testing.T) {
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
	addWorkingSetUserTurns(t, app, session.ID, 2)

	first, _, err := app.loadConversationTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	first.workingSetSessionID = session.ID
	first.workingSetTurn = 1
	if _, err := app.discoverConversationTools(context.Background(), first, map[string]any{"resources": []string{"calendar"}}); err != nil {
		t.Fatal(err)
	}

	second, _, err := app.loadConversationTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	second.workingSetSessionID = session.ID
	second.workingSetTurn = 2
	if restored := app.restoreConversationToolWorkingSet(context.Background(), session.ID, 2, second); restored != 1 {
		t.Fatalf("restored tools = %d, want 1", restored)
	}
	if !second.exposure.Has("calendar__events_update") {
		t.Fatal("next turn should expose the previously discovered tool before model execution")
	}
	if got := calendar.listCalls.Load(); got != 1 {
		t.Fatalf("next turn should reuse current cached catalog, tools/list calls = %d", got)
	}
}

func TestConversationToolWorkingSetPrioritizesCallsAndPrunesToBudget(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"demo": {URL: "http://example.invalid", ToolExposure: mcp.ToolExposureOnDemand},
	}}
	app := newResourceTestApp(t, cfg)
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	addWorkingSetUserTurns(t, app, session.ID, 1)

	tools := make([]mcp.MCPTool, 0, 10)
	entries := make([]storepkg.SessionToolWorkingSetEntry, 0, 10)
	for i := 0; i < 10; i++ {
		name := "tool_" + string(rune('a'+i))
		fullName := "demo__" + name
		tools = append(tools, mcp.MCPTool{Server: "demo", Name: name, FullName: fullName, InputSchema: map[string]any{"type": "object"}})
		entries = append(entries, storepkg.SessionToolWorkingSetEntry{ToolName: fullName, ResourceID: "demo"})
	}
	if err := app.store.RecordSessionToolDiscovery(session.ID, 1, entries); err != nil {
		t.Fatal(err)
	}
	if err := app.store.RecordSessionToolCall(session.ID, 1, entries[9]); err != nil {
		t.Fatal(err)
	}

	set := newConversationToolSet(tools, cfg)
	if restored := app.restoreConversationToolWorkingSet(context.Background(), session.ID, 2, set); restored != maxConversationStickyTools {
		t.Fatalf("restored tools = %d, want %d", restored, maxConversationStickyTools)
	}
	if !set.exposure.Has(entries[9].ToolName) {
		t.Fatalf("actually called tool must win sticky budget: %#v", set.exposure.names)
	}
	stored, err := app.store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != maxConversationStickyTools {
		t.Fatalf("persisted working set should be pruned to %d, got %#v", maxConversationStickyTools, stored)
	}
}

func TestConversationToolWorkingSetBudgetStopsLoadingExtraResources(t *testing.T) {
	app := newResourceTestApp(t, mcp.MCPConfig{})
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	addWorkingSetUserTurns(t, app, session.ID, 1)
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{}}
	entries := make([]storepkg.SessionToolWorkingSetEntry, 0, maxConversationStickyTools+1)
	for i := 0; i < maxConversationStickyTools+1; i++ {
		resourceID := fmt.Sprintf("resource_%02d", i)
		cfg.Servers[resourceID] = mcp.MCPServerConfig{URL: "http://example.invalid", ToolExposure: mcp.ToolExposureOnDemand}
		entries = append(entries, storepkg.SessionToolWorkingSetEntry{ToolName: resourceID + "__use", ResourceID: resourceID})
	}
	if err := app.store.RecordSessionToolDiscovery(session.ID, 1, entries); err != nil {
		t.Fatal(err)
	}

	set := newConversationToolSet(nil, cfg)
	loads := 0
	set.resources.loader = func(_ context.Context, resourceID string) ([]mcp.MCPTool, error) {
		loads++
		return []mcp.MCPTool{{
			Server: resourceID, Name: "use", FullName: resourceID + "__use", InputSchema: map[string]any{"type": "object"},
		}}, nil
	}
	if restored := app.restoreConversationToolWorkingSet(context.Background(), session.ID, 2, set); restored != maxConversationStickyTools {
		t.Fatalf("restored tools = %d, want %d", restored, maxConversationStickyTools)
	}
	if loads != maxConversationStickyTools {
		t.Fatalf("working-set budget should stop tools/list after %d resources, got %d", maxConversationStickyTools, loads)
	}
}

func TestConversationToolWorkingSetDropsToolMissingFromCurrentCatalog(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"demo": {URL: "http://example.invalid", ToolExposure: mcp.ToolExposureOnDemand},
	}}
	app := newResourceTestApp(t, cfg)
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	addWorkingSetUserTurns(t, app, session.ID, 1)
	old := storepkg.SessionToolWorkingSetEntry{ToolName: "demo__old", ResourceID: "demo"}
	if err := app.store.RecordSessionToolCall(session.ID, 1, old); err != nil {
		t.Fatal(err)
	}

	set := newConversationToolSet([]mcp.MCPTool{{
		Server: "demo", Name: "new", FullName: "demo__new", InputSchema: map[string]any{"type": "object"},
	}}, cfg)
	if restored := app.restoreConversationToolWorkingSet(context.Background(), session.ID, 2, set); restored != 0 {
		t.Fatalf("removed tool must not be restored, got %d", restored)
	}
	stored, err := app.store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("missing live catalog entry should be pruned: %#v", stored)
	}
}

func TestToolWorkingSetTurnRetention(t *testing.T) {
	if !toolWorkingSetTurnIsRecent(1, 7, calledToolRetentionTurns) || toolWorkingSetTurnIsRecent(1, 8, calledToolRetentionTurns) {
		t.Fatal("called tool retention must include six following turns and expire on the seventh")
	}
	if !toolWorkingSetTurnIsRecent(3, 5, discoveredToolRetentionTurns) || toolWorkingSetTurnIsRecent(3, 6, discoveredToolRetentionTurns) {
		t.Fatal("discovery retention must include two following turns and expire on the third")
	}
	if toolWorkingSetTurnIsRecent(5, 4, calledToolRetentionTurns) {
		t.Fatal("future turn from a concurrent request must never be restored by an older request")
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
		t.Fatalf("successful call should become strong sticky: %#v", entries)
	}
}
