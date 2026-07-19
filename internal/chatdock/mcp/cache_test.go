package mcp

import (
	"fmt"
	"testing"
	"time"
)

func TestToolCacheDeepCopiesNestedSchemas(t *testing.T) {
	client := NewMCPClient()
	server := MCPServerConfig{CacheTTLMS: 60_000}
	tools := []MCPTool{{
		FullName: "demo__create",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string", "enum": []any{"a", "b"}},
			},
			"required": []string{"title"},
		},
	}}
	client.storeCachedTools("demo", tools)

	tools[0].InputSchema["type"] = "mutated"
	properties := tools[0].InputSchema["properties"].(map[string]any)
	properties["title"].(map[string]any)["type"] = "number"
	tools[0].InputSchema["required"].([]string)[0] = "other"

	first, ok := client.cachedTools("demo", server)
	if !ok {
		t.Fatal("expected cached tools")
	}
	assertCachedToolSchemaUntouched(t, first[0])

	first[0].InputSchema["type"] = "changed-again"
	first[0].InputSchema["properties"].(map[string]any)["title"].(map[string]any)["enum"].([]any)[0] = "changed"
	second, ok := client.cachedTools("demo", server)
	if !ok {
		t.Fatal("expected cached tools after caller mutation")
	}
	assertCachedToolSchemaUntouched(t, second[0])
}

func assertCachedToolSchemaUntouched(t *testing.T, tool MCPTool) {
	t.Helper()
	if tool.InputSchema["type"] != "object" {
		t.Fatalf("schema type = %#v", tool.InputSchema["type"])
	}
	properties := tool.InputSchema["properties"].(map[string]any)
	title := properties["title"].(map[string]any)
	if title["type"] != "string" || title["enum"].([]any)[0] != "a" {
		t.Fatalf("nested schema was mutated: %#v", tool.InputSchema)
	}
	if tool.InputSchema["required"].([]string)[0] != "title" {
		t.Fatalf("required fields were mutated: %#v", tool.InputSchema["required"])
	}
}

func TestToolCacheEvictsOldestEntryAtCapacity(t *testing.T) {
	client := NewMCPClient()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < maxMCPToolCacheEntries; i++ {
		client.toolsCache[fmt.Sprintf("key-%03d", i)] = cachedMCPTools{createdAt: base.Add(time.Duration(i) * time.Second)}
	}
	client.storeCachedTools("new-key", []MCPTool{{FullName: "demo__new"}})

	if len(client.toolsCache) != maxMCPToolCacheEntries {
		t.Fatalf("cache size = %d, want %d", len(client.toolsCache), maxMCPToolCacheEntries)
	}
	if _, ok := client.toolsCache["key-000"]; ok {
		t.Fatal("oldest cache entry was not evicted")
	}
	if _, ok := client.toolsCache["new-key"]; !ok {
		t.Fatal("new cache entry was not stored")
	}
}

func TestToolCacheRefreshAtCapacityDoesNotEvictAnotherEntry(t *testing.T) {
	client := NewMCPClient()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < maxMCPToolCacheEntries; i++ {
		client.toolsCache[fmt.Sprintf("key-%03d", i)] = cachedMCPTools{createdAt: base.Add(time.Duration(i) * time.Second)}
	}
	client.storeCachedTools("key-127", []MCPTool{{FullName: "demo__updated"}})

	if len(client.toolsCache) != maxMCPToolCacheEntries {
		t.Fatalf("cache size = %d, want %d", len(client.toolsCache), maxMCPToolCacheEntries)
	}
	if _, ok := client.toolsCache["key-000"]; !ok {
		t.Fatal("refreshing an existing key should not evict another entry")
	}
	if got := client.toolsCache["key-127"].tools[0].FullName; got != "demo__updated" {
		t.Fatalf("refreshed tool = %q", got)
	}
}

func TestToolCacheRemovesExpiredEntry(t *testing.T) {
	client := NewMCPClient()
	client.toolsCache["expired"] = cachedMCPTools{createdAt: time.Now().Add(-time.Second)}
	if _, ok := client.cachedTools("expired", MCPServerConfig{CacheTTLMS: 1}); ok {
		t.Fatal("expired entry should miss")
	}
	if _, ok := client.toolsCache["expired"]; ok {
		t.Fatal("expired entry should be removed")
	}
}
