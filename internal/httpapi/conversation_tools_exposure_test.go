package httpapi

import (
	"context"
	"strings"
	"testing"

	"chatdock/internal/mcp"
)

func TestConversationToolSetAppliesGenericServerDefaultAndToolOverride(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {
			ToolExposure: mcp.ToolExposureOnDemand,
			ToolOverrides: map[string]mcp.ToolExposure{
				"calendar__events_list": mcp.ToolExposureDirect,
			},
		},
	}}
	tools := []mcp.MCPTool{
		{Server: "calendar", Name: "events_list", FullName: "calendar__events_list", Title: "查询日历"},
		{Server: "calendar", Name: "events_create", FullName: "calendar__events_create", Title: "创建日历事件"},
	}

	set := newConversationToolSet(tools, cfg)

	if _, ok := set.visibleTool("calendar__events_list"); !ok {
		t.Fatal("direct override should expose the real tool immediately")
	}
	if _, ok := set.visibleTool("calendar__events_create"); ok {
		t.Fatal("server default on_demand should hide the real tool initially")
	}
	if _, ok := set.visibleTool(builtinToolSearchTools); !ok {
		t.Fatal("on-demand tools should expose the search entrypoint")
	}
	if len(set.onDemand.tools) != 1 || set.onDemand.tools[0].FullName != "calendar__events_create" {
		t.Fatalf("unexpected on-demand catalog: %#v", set.onDemand.tools)
	}
}

func TestConversationToolSetCanHideOneToolFromDirectServer(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {
			ToolExposure: mcp.ToolExposureDirect,
			ToolOverrides: map[string]mcp.ToolExposure{
				"events_delete": mcp.ToolExposureOnDemand,
			},
		},
	}}
	tools := []mcp.MCPTool{
		{Server: "calendar", Name: "events_list", FullName: "calendar__events_list", Title: "查询日历"},
		{Server: "calendar", Name: "events_delete", FullName: "calendar__events_delete", Title: "删除日历事件"},
	}

	set := newConversationToolSet(tools, cfg)
	if _, ok := set.visibleTool("calendar__events_list"); !ok {
		t.Fatal("server default direct should expose tools without overrides")
	}
	if _, ok := set.visibleTool("calendar__events_delete"); ok {
		t.Fatal("on_demand override should hide the selected tool initially")
	}
	if _, ok := set.visibleTool(builtinToolSearchTools); !ok {
		t.Fatal("a hidden override should keep the search entrypoint available")
	}
}

func TestSearchingOnDemandToolsExposesRealToolForDirectCall(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {ToolExposure: mcp.ToolExposureOnDemand},
	}}
	tools := []mcp.MCPTool{{
		Server:      "calendar",
		Name:        "events_create",
		FullName:    "calendar__events_create",
		Title:       "创建日历事件",
		Description: "创建新的日历安排",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}}, "required": []string{"title"}},
	}}
	set := newConversationToolSet(tools, cfg)
	app := &Server{}
	called := ""
	runRealTool := func(name string, args map[string]any) (any, error) {
		called = name
		return args, nil
	}

	result, err := app.callVisibleConversationTool(context.Background(), set, runRealTool, builtinToolSearchTools, map[string]any{"query": "创建日历"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := result.(map[string]any)["loaded_tools"].([]string)
	if !ok || len(loaded) != 1 || loaded[0] != "calendar__events_create" {
		t.Fatalf("search should expose the matched real tool, got %#v", result)
	}

	if _, err := app.callVisibleConversationTool(context.Background(), set, runRealTool, "calendar__events_create", map[string]any{"title": "周会"}); err != nil {
		t.Fatal(err)
	}
	if called != "calendar__events_create" {
		t.Fatalf("expected direct real tool call, got %q", called)
	}
}

func TestToolSearchUsesDeclaredSchemaValidation(t *testing.T) {
	cfg := mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"calendar": {ToolExposure: mcp.ToolExposureOnDemand},
	}}
	set := newConversationToolSet([]mcp.MCPTool{{
		Server:      "calendar",
		Name:        "events_list",
		FullName:    "calendar__events_list",
		Description: "查询日历安排",
	}}, cfg)
	app := &Server{}
	runRealTool := func(name string, args map[string]any) (any, error) {
		t.Fatalf("discovery validation must run before real tool execution: %s %#v", name, args)
		return nil, nil
	}

	_, err := app.callVisibleConversationTool(context.Background(), set, runRealTool, builtinToolSearchTools, map[string]any{
		"query": "日历",
		"limit": "8",
	})
	if err == nil || !strings.Contains(err.Error(), "arguments.limit must be integer") {
		t.Fatalf("expected schema type validation for discovery tool, got %v", err)
	}

	_, err = app.callVisibleConversationTool(context.Background(), set, runRealTool, builtinToolSearchTools, map[string]any{
		"query":      "日历",
		"unexpected": true,
	})
	if err == nil || !strings.Contains(err.Error(), "arguments.unexpected is not allowed") {
		t.Fatalf("expected additionalProperties validation for discovery tool, got %v", err)
	}
}

func TestConversationToolSetAppliesBuiltinDefaultAndToolOverride(t *testing.T) {
	cfg := mcp.MCPConfig{BuiltinTools: mcp.ToolExposureConfig{
		ToolExposure: mcp.ToolExposureOnDemand,
		ToolOverrides: map[string]mcp.ToolExposure{
			builtinToolCreateScheduledTask: mcp.ToolExposureDirect,
		},
	}}
	set := newConversationToolSet(builtinScheduledTaskTools(), cfg)

	if _, ok := set.visibleTool(builtinToolCreateScheduledTask); !ok {
		t.Fatal("direct builtin override should expose the real tool immediately")
	}
	if _, ok := set.visibleTool(builtinToolListScheduledTasks); ok {
		t.Fatal("builtin on_demand default should hide tools without overrides")
	}
	if _, ok := set.visibleTool(builtinToolSearchTools); !ok {
		t.Fatal("hidden builtin tools should expose the search entrypoint")
	}
}

func TestSearchingOnDemandBuiltinToolExposesItForDirectCall(t *testing.T) {
	cfg := mcp.MCPConfig{BuiltinTools: mcp.ToolExposureConfig{ToolExposure: mcp.ToolExposureOnDemand}}
	set := newConversationToolSet(builtinScheduledTaskTools(), cfg)
	app := &Server{}
	called := ""
	runRealTool := func(name string, args map[string]any) (any, error) {
		called = name
		return args, nil
	}

	result, err := app.callVisibleConversationTool(context.Background(), set, runRealTool, builtinToolSearchTools, map[string]any{"query": "查询定时任务"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := result.(map[string]any)["loaded_tools"].([]string)
	if !ok || len(loaded) == 0 {
		t.Fatalf("search should expose a matched builtin tool, got %#v", result)
	}

	if _, err := app.callVisibleConversationTool(context.Background(), set, runRealTool, builtinToolListScheduledTasks, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if called != builtinToolListScheduledTasks {
		t.Fatalf("expected direct builtin tool call after discovery, got %q", called)
	}
}
