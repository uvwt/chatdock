package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const maxMCPToolCacheEntries = 128

// CachedServerTools 返回资源近期缓存的完整工具定义，调用方可以安全修改返回值。
func (c *MCPClient) CachedServerTools(cfg MCPConfig, serverName string) ([]MCPTool, bool) {
	server, ok := cfg.Servers[serverName]
	if !ok || server.Disabled || strings.TrimSpace(server.URL) == "" {
		return nil, false
	}
	return c.cachedTools(serverCacheKey(serverName, server), server)
}

func (c *MCPClient) cachedTools(key string, server MCPServerConfig) ([]MCPTool, bool) {
	ttl := server.cacheTTL()
	if ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.toolsCache[key]
	if !ok {
		return nil, false
	}
	if time.Since(item.createdAt) > ttl {
		delete(c.toolsCache, key)
		return nil, false
	}
	return cloneTools(item.tools), true
}

func (c *MCPClient) storeCachedTools(key string, tools []MCPTool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.toolsCache[key]; !exists && len(c.toolsCache) >= maxMCPToolCacheEntries {
		oldestKey := ""
		var oldestAt time.Time
		for existingKey, item := range c.toolsCache {
			if oldestKey == "" || item.createdAt.Before(oldestAt) {
				oldestKey = existingKey
				oldestAt = item.createdAt
			}
		}
		delete(c.toolsCache, oldestKey)
	}
	c.toolsCache[key] = cachedMCPTools{createdAt: time.Now(), tools: cloneTools(tools)}
}

func (c *MCPClient) invalidateToolsCache(key string) {
	c.mu.Lock()
	delete(c.toolsCache, key)
	c.mu.Unlock()
}

func (s MCPServerConfig) cacheTTL() time.Duration {
	if s.CacheTTLMS < 0 {
		return 0
	}
	if s.CacheTTLMS > 0 {
		return time.Duration(s.CacheTTLMS) * time.Millisecond
	}
	return 30 * time.Second
}

func serverCacheKey(serverName string, server MCPServerConfig) string {
	raw, _ := json.Marshal(server)
	sum := sha256.Sum256(append([]byte(serverName+":"), raw...))
	return hex.EncodeToString(sum[:])
}

func cloneTools(tools []MCPTool) []MCPTool {
	out := make([]MCPTool, len(tools))
	for i, tool := range tools {
		out[i] = tool
		out[i].InputSchema = cloneJSONMap(tool.InputSchema)
		out[i].OutputSchema = cloneJSONMap(tool.OutputSchema)
		out[i].Annotations = cloneJSONMap(tool.Annotations)
		out[i].Meta = cloneJSONMap(tool.Meta)
		if tool.Icons != nil {
			out[i].Icons = make([]map[string]any, len(tool.Icons))
			for j, icon := range tool.Icons {
				out[i].Icons[j] = cloneJSONMap(icon)
			}
		}
	}
	return out
}

func cloneJSONMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneJSONValue(item)
	}
	return out
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneJSONValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}
