package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

func (c *MCPClient) cachedTools(key string, server MCPServerConfig) ([]MCPTool, bool) {
	ttl := server.cacheTTL()
	if ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.toolsCache[key]
	if !ok || time.Since(item.createdAt) > ttl {
		return nil, false
	}
	return cloneTools(item.tools), true
}

func (c *MCPClient) storeCachedTools(key string, tools []MCPTool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.toolsCache[key] = cachedMCPTools{createdAt: time.Now(), tools: cloneTools(tools)}
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
	copy(out, tools)
	return out
}
