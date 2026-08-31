package httpapi

import (
	"context"
	"sort"
	"strings"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

const (
	maxConversationStickyTools   = 8
	calledToolRetentionTurns     = 6
	discoveredToolRetentionTurns = 2
)

type conversationStickyTool struct {
	entry        storepkg.SessionToolWorkingSetEntry
	calledRecent bool
}

func conversationUserTurn(history []model.Message) int {
	turn := 0
	for _, message := range history {
		if strings.TrimSpace(message.Role) == "user" {
			turn++
		}
	}
	return turn
}

// restoreConversationToolWorkingSet 只把近期使用倾向映射回当前 catalog。
// 持久化状态从不携带 Schema，也不能绕过当前 exposure/allow 规则复活旧工具。
func (a *Server) restoreConversationToolWorkingSet(ctx context.Context, sessionID string, turn int, toolSet *conversationToolSet) int {
	if a.store == nil || toolSet == nil || strings.TrimSpace(sessionID) == "" || turn <= 0 {
		return 0
	}
	entries, err := a.store.SessionToolWorkingSet(sessionID)
	if err != nil {
		logError("tool_working_set_load_failed", err, logFields{"session_id": sessionID})
		return 0
	}

	candidates := make([]conversationStickyTool, 0, len(entries))
	removeEntries := make([]storepkg.SessionToolWorkingSetEntry, 0)
	for _, entry := range entries {
		calledRecent := toolWorkingSetTurnIsRecent(entry.LastCalledTurn, turn, calledToolRetentionTurns)
		discoveredRecent := toolWorkingSetTurnIsRecent(entry.LastDiscoveredTurn, turn, discoveredToolRetentionTurns)
		if !calledRecent && !discoveredRecent {
			// future turn 可能来自同一会话的并发请求，不能被较旧请求误删。
			if entry.LastCalledTurn <= turn && entry.LastDiscoveredTurn <= turn {
				removeEntries = append(removeEntries, entry)
			}
			continue
		}

		resource := toolSet.resources.byID[entry.ResourceID]
		if resource == nil || resource.info.Status == "disabled" {
			removeEntries = append(removeEntries, entry)
			continue
		}
		candidates = append(candidates, conversationStickyTool{entry: entry, calledRecent: calledRecent})
	}

	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.calledRecent != right.calledRecent {
			return left.calledRecent
		}
		if left.entry.LastCalledTurn != right.entry.LastCalledTurn {
			return left.entry.LastCalledTurn > right.entry.LastCalledTurn
		}
		if left.entry.LastDiscoveredTurn != right.entry.LastDiscoveredTurn {
			return left.entry.LastDiscoveredTurn > right.entry.LastDiscoveredTurn
		}
		return left.entry.ToolName < right.entry.ToolName
	})
	// 先按轻量 metadata 排序，再逐个向 live catalog 解析；凑满预算后不再加载其他资源。
	// 这样 max 8 不只限制模型 Schema，也限制跨资源 tools/list 成本。
	tools := make([]mcp.MCPTool, 0, min(len(candidates), maxConversationStickyTools))
	loadAttempted := map[string]bool{}
	loadErrors := map[string]error{}
	for _, candidate := range candidates {
		if len(tools) >= maxConversationStickyTools {
			removeEntries = append(removeEntries, candidate.entry)
			continue
		}
		entry := candidate.entry
		resource := toolSet.resources.byID[entry.ResourceID]
		if !resource.info.Loaded {
			if !loadAttempted[entry.ResourceID] {
				loadAttempted[entry.ResourceID] = true
				_, loadErrors[entry.ResourceID] = toolSet.loadResource(ctx, entry.ResourceID)
			}
			if loadErrors[entry.ResourceID] != nil {
				// 网络或远端临时错误不代表工具已失效，本轮跳过但保留 affinity。
				continue
			}
		}

		tool, ok := toolSet.loaded.Get(entry.ToolName)
		if !ok {
			removeEntries = append(removeEntries, entry)
			continue
		}
		if _, onDemand := toolSet.onDemand.Get(entry.ToolName); !onDemand {
			// 已改为 direct 的工具无需继续占用 sticky budget。
			removeEntries = append(removeEntries, entry)
			continue
		}
		tools = append(tools, tool)
	}
	loaded := toolSet.expose(tools)
	if err := a.store.DeleteSessionToolWorkingSetEntriesIfUnchanged(sessionID, removeEntries); err != nil {
		logError("tool_working_set_prune_failed", err, logFields{"session_id": sessionID})
	}
	return len(loaded)
}

func toolWorkingSetTurnIsRecent(lastTurn int, currentTurn int, retention int) bool {
	if lastTurn <= 0 || lastTurn > currentTurn {
		return false
	}
	return currentTurn-lastTurn <= retention
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
