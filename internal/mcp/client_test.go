package mcp

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseMCPConfigAndToolFilters(t *testing.T) {
	content := `{
		"servers": {
			"agentdock": {
				"url": "http://127.0.0.1:18766/mcp",
				"allow_tools": ["recall_*"],
				"deny_tools": ["recall_delete"],
				"confirm_tools": ["recall_write"]
			}
		}
	}`
	cfg, err := ParseMCPConfig(content)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Servers["agentdock"]
	if !s.allowsTool("recall_read", "agentdock__recall_read") {
		t.Fatal("expected recall_read to be allowed")
	}
	if s.allowsTool("exec_command", "agentdock__exec_command") {
		t.Fatal("expected exec_command to be filtered by allow list")
	}
	if s.allowsTool("recall_delete", "agentdock__recall_delete") {
		t.Fatal("expected recall_delete to be denied")
	}
	if !s.requiresConfirmation("recall_write", "agentdock__recall_write") {
		t.Fatal("expected recall_write to require confirmation")
	}
}

func TestNormalizeBearerToken(t *testing.T) {
	if got := normalizeBearerToken("Bearer abc"); got != "abc" {
		t.Fatalf("expected prefix to be stripped, got %q", got)
	}
	if got := normalizeBearerToken("  bearer abc  "); got != "abc" {
		t.Fatalf("expected lowercase prefix to be stripped, got %q", got)
	}
	if got := normalizeBearerToken("abc"); got != "abc" {
		t.Fatalf("expected raw value to stay unchanged, got %q", got)
	}
}

func TestToolExposureUsesServerDefaultAndPerToolOverrides(t *testing.T) {
	server := MCPServerConfig{
		ToolExposure: ToolExposureOnDemand,
		ToolOverrides: map[string]ToolExposure{
			"events_list":             ToolExposureDirect,
			"calendar__events_delete": ToolExposureDirect,
			"events_update":           ToolExposureInherit,
		},
	}

	if got := server.ExposureForTool("events_list", "calendar__events_list"); got != ToolExposureDirect {
		t.Fatalf("tool name override should win, got %q", got)
	}
	if got := server.ExposureForTool("events_delete", "calendar__events_delete"); got != ToolExposureDirect {
		t.Fatalf("full name override should win, got %q", got)
	}
	if got := server.ExposureForTool("events_update", "calendar__events_update"); got != ToolExposureOnDemand {
		t.Fatalf("inherit should use server default, got %q", got)
	}
	if got := (MCPServerConfig{}).ExposureForTool("events_list", "calendar__events_list"); got != ToolExposureOnDemand {
		t.Fatalf("missing exposure should default to on_demand, got %q", got)
	}
	directServer := MCPServerConfig{
		ToolExposure:  ToolExposureDirect,
		ToolOverrides: map[string]ToolExposure{"events_delete": ToolExposureOnDemand},
	}
	if got := directServer.ExposureForTool("events_delete", "calendar__events_delete"); got != ToolExposureOnDemand {
		t.Fatalf("on_demand override should hide a tool from a direct server, got %q", got)
	}
}

func TestBuiltinToolExposureDefaultsToDirectAndSupportsOverrides(t *testing.T) {
	if got := (ToolExposureConfig{}).ExposureForTool("scheduled_tasks_list", "chatdock_scheduled_tasks_list"); got != ToolExposureDirect {
		t.Fatalf("missing builtin exposure should preserve direct loading, got %q", got)
	}

	config := ToolExposureConfig{
		ToolExposure: ToolExposureOnDemand,
		ToolOverrides: map[string]ToolExposure{
			"chatdock_scheduled_task_create": ToolExposureDirect,
		},
	}
	if got := config.ExposureForTool("scheduled_tasks_list", "chatdock_scheduled_tasks_list"); got != ToolExposureOnDemand {
		t.Fatalf("builtin default should apply to tools without overrides, got %q", got)
	}
	if got := config.ExposureForTool("scheduled_task_create", "chatdock_scheduled_task_create"); got != ToolExposureDirect {
		t.Fatalf("builtin full-name override should win, got %q", got)
	}
}

func TestParseMCPConfigRejectsInvalidToolExposure(t *testing.T) {
	tests := []string{
		`{"builtin_tools":{"tool_exposure":"automatic"},"servers":{}}`,
		`{"builtin_tools":{"tool_overrides":{"chatdock_scheduled_tasks_list":"automatic"}},"servers":{}}`,
		`{"servers":{"calendar":{"tool_exposure":"automatic"}}}`,
		`{"servers":{"calendar":{"tool_overrides":{"events_list":"automatic"}}}}`,
	}
	for _, content := range tests {
		if _, err := ParseMCPConfig(content); err == nil {
			t.Fatalf("expected invalid exposure to fail: %s", content)
		}
	}
}

func TestParseMCPConfigRejectsBuiltInResourceName(t *testing.T) {
	for _, name := range []string{"ChatDock", "chatdock", " CHATDOCK "} {
		content := fmt.Sprintf(`{"servers":{%q:{"url":"http://example.test/mcp"}}}`, name)
		if _, err := ParseMCPConfig(content); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("reserved resource name %q should be rejected, got %v", name, err)
		}
	}
}
