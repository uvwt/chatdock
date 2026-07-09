package mcp

import "testing"

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
