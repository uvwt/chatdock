package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ToolFullName(serverName, toolName string) string {
	return toolFullName(serverName, toolName)
}

func CompactJSON(value any) string {
	return compactJSON(value)
}

func NormalizeJSONSchema(schema map[string]any) map[string]any {
	return normalizeJSONSchema(schema)
}

func toolFullName(serverName, toolName string) string {
	return safeToolName(serverName) + "__" + safeToolName(toolName)
}

func SplitToolFullName(fullName string) (string, string) {
	return splitToolFullName(fullName)
}

func ResolveToolServer(cfg MCPConfig, fullName string) (string, string, MCPServerConfig, error) {
	serverAlias, toolName := splitToolFullName(fullName)
	var matchedName string
	var matchedServer MCPServerConfig
	matchCount := 0
	for serverName, server := range cfg.Servers {
		if safeToolName(serverName) != serverAlias {
			continue
		}
		matchedName = serverName
		matchedServer = server
		matchCount++
	}
	switch matchCount {
	case 0:
		return "", "", MCPServerConfig{}, fmt.Errorf("mcp server not found for alias: %s", serverAlias)
	case 1:
		return matchedName, toolName, matchedServer, nil
	default:
		return "", "", MCPServerConfig{}, fmt.Errorf("mcp server alias is ambiguous: %s", serverAlias)
	}
}

func splitToolFullName(fullName string) (string, string) {
	parts := strings.SplitN(fullName, "__", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "agentdock", fullName
}

func safeToolName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "tool"
	}
	return b.String()
}

func normalizeJSONSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	out := make(map[string]any, len(schema)+2)
	for k, v := range schema {
		out[k] = v
	}
	if _, ok := out["type"]; !ok {
		out["type"] = "object"
	}
	if _, ok := out["properties"]; !ok {
		out["properties"] = map[string]any{}
	}
	return out
}

func compactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(raw)
}
