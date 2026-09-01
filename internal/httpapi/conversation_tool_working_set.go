package httpapi

import (
	"strings"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

const (
	maxConversationStickyTools = 8
)

func conversationUserTurn(history []model.Message) int {
	turn := 0
	for _, message := range history {
		if strings.TrimSpace(message.Role) == "user" {
			turn++
		}
	}
	return turn
}

func (a *Server) rememberDiscoveredConversationTools(sessionID string, turn int, toolSet *conversationToolSet, tools []mcp.MCPTool) {
	if a.store == nil || toolSet == nil || turn <= 0 || len(tools) == 0 {
		return
	}
	entries := make([]storepkg.SessionToolWorkingSetEntry, 0, min(len(tools), maxConversationStickyTools))
	seen := map[string]bool{}
	for _, tool := range tools {
		if len(entries) >= maxConversationStickyTools {
			break
		}
		if seen[tool.FullName] {
			continue
		}
		if _, onDemand := toolSet.onDemand.Get(tool.FullName); !onDemand {
			continue
		}
		seen[tool.FullName] = true
		entries = append(entries, storepkg.SessionToolWorkingSetEntry{
			ToolName:   tool.FullName,
			ResourceID: conversationToolResourceID(tool),
		})
	}
	if err := a.store.RecordSessionToolDiscovery(sessionID, turn, entries); err != nil {
		logError("tool_working_set_discovery_record_failed", err, logFields{"session_id": sessionID})
	}
}

func (a *Server) rememberCalledConversationTool(sessionID string, turn int, toolSet *conversationToolSet, tool mcp.MCPTool) {
	if a.store == nil || toolSet == nil || turn <= 0 {
		return
	}
	if _, onDemand := toolSet.onDemand.Get(tool.FullName); !onDemand {
		return
	}
	entry := storepkg.SessionToolWorkingSetEntry{
		ToolName:   tool.FullName,
		ResourceID: conversationToolResourceID(tool),
	}
	if err := a.store.RecordSessionToolCall(sessionID, turn, entry); err != nil {
		logError("tool_working_set_call_record_failed", err, logFields{"session_id": sessionID, "tool": tool.FullName})
	}
}

func conversationToolResourceID(tool mcp.MCPTool) string {
	if tool.Server == builtinToolServerDiscovery {
		return builtinToolResourceID
	}
	return tool.Server
}
