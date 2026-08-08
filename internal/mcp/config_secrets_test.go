package mcp

import (
	"strings"
	"testing"
)

const currentSecretConfig = `{
  "builtin_tools": {"tool_exposure": "direct"},
  "future_root_field": {"enabled": true},
  "servers": {
    "DockMini": {
      "url": "http://agentdock.test/mcp",
      "future_server_field": "kept",
      "auth": {"type": "bearer", "token": "old-secret", "future_auth_field": "kept"}
    }
  }
}`

func TestRedactConfigTokensRemovesSecretAndKeepsReference(t *testing.T) {
	redacted, err := RedactConfigTokens(currentSecretConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(redacted, "old-secret") {
		t.Fatal("redacted MCP config still contains the saved token")
	}
	for _, want := range []string{`"_chatdock_token_ref": "DockMini"`, `"future_root_field"`, `"future_server_field"`, `"future_auth_field"`} {
		if !strings.Contains(redacted, want) {
			t.Fatalf("redacted MCP config lost %s: %s", want, redacted)
		}
	}
}

func TestMergeConfigTokensPreservesReplacesAndClears(t *testing.T) {
	t.Run("preserve", func(t *testing.T) {
		merged := mergeConfigForTest(t, currentSecretConfig, `{
          "servers": {"DockMini": {"url": "http://changed.test/mcp", "auth": {"type": "bearer", "_chatdock_token_ref": "DockMini"}}}
        }`)
		server := merged.Servers["DockMini"]
		if server.Auth.Token != "old-secret" || server.URL != "http://changed.test/mcp" {
			t.Fatalf("preserved server = %#v", server)
		}
	})

	t.Run("rename", func(t *testing.T) {
		merged := mergeConfigForTest(t, currentSecretConfig, `{
          "servers": {"Local": {"url": "http://agentdock.test/mcp", "auth": {"type": "bearer", "_chatdock_token_ref": "DockMini"}}}
        }`)
		if merged.Servers["Local"].Auth.Token != "old-secret" {
			t.Fatalf("renamed token = %q", merged.Servers["Local"].Auth.Token)
		}
	})

	t.Run("replace", func(t *testing.T) {
		merged := mergeConfigForTest(t, currentSecretConfig, `{
          "servers": {"DockMini": {"url": "http://agentdock.test/mcp", "auth": {"type": "bearer", "token": "  Bearer new-secret  ", "_chatdock_token_ref": "DockMini"}}}
        }`)
		if merged.Servers["DockMini"].Auth.Token != "new-secret" {
			t.Fatalf("replacement token = %q", merged.Servers["DockMini"].Auth.Token)
		}
	})

	t.Run("clear", func(t *testing.T) {
		merged := mergeConfigForTest(t, currentSecretConfig, `{
          "servers": {"DockMini": {"url": "http://agentdock.test/mcp", "auth": {"type": "bearer"}}}
        }`)
		if merged.Servers["DockMini"].Auth.Token != "" {
			t.Fatalf("cleared token = %q", merged.Servers["DockMini"].Auth.Token)
		}
	})
}

func TestMergeConfigTokensRejectsUnavailableOrReusedReferences(t *testing.T) {
	for name, submitted := range map[string]string{
		"missing": `{"servers":{"DockMini":{"auth":{"type":"bearer","_chatdock_token_ref":"missing"}}}}`,
		"reused":  `{"servers":{"One":{"auth":{"type":"bearer","_chatdock_token_ref":"DockMini"}},"Two":{"auth":{"type":"bearer","_chatdock_token_ref":"DockMini"}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := MergeConfigTokens(currentSecretConfig, submitted); err == nil {
				t.Fatal("expected invalid token reference to fail")
			}
		})
	}
}

func mergeConfigForTest(t *testing.T, current, submitted string) MCPConfig {
	t.Helper()
	raw, err := MergeConfigTokens(current, submitted)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, mcpTokenReferenceField) {
		t.Fatalf("merged config persisted draft reference: %s", raw)
	}
	config, err := ParseMCPConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	return config
}
