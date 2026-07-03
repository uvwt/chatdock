package mcp

import "testing"

func TestParseMCPConfigAndToolFilters(t *testing.T) {
	content := `{
		"servers": {
			"agentdock": {
				"url": "http://127.0.0.1:18766/mcp",
				"allow_tools": ["memory_*"],
				"deny_tools": ["memory_delete"],
				"confirm_tools": ["memory_write"]
			}
		}
	}`
	cfg, err := ParseMCPConfig(content)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Servers["agentdock"]
	if !s.allowsTool("memory_read", "agentdock__memory_read") {
		t.Fatal("expected memory_read to be allowed")
	}
	if s.allowsTool("desktop_click", "agentdock__desktop_click") {
		t.Fatal("expected desktop_click to be filtered by allow list")
	}
	if s.allowsTool("memory_delete", "agentdock__memory_delete") {
		t.Fatal("expected memory_delete to be denied")
	}
	if !s.requiresConfirmation("memory_write", "agentdock__memory_write") {
		t.Fatal("expected memory_write to require confirmation")
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
