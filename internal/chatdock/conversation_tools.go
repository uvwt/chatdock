package chatdock

import (
	"context"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

type conversationToolSet struct {
	visible      []mcp.MCPTool
	visibleNames map[string]bool
	allByName    map[string]mcp.MCPTool
	onDemand     toolCatalog
}

func newConversationToolSet(allTools []mcp.MCPTool, cfg mcp.MCPConfig) *conversationToolSet {
	set := &conversationToolSet{
		visible:      make([]mcp.MCPTool, 0, len(allTools)+1),
		visibleNames: map[string]bool{},
		allByName:    map[string]mcp.MCPTool{},
	}
	onDemandTools := make([]mcp.MCPTool, 0)
	for _, tool := range allTools {
		if strings.TrimSpace(tool.FullName) == "" {
			tool.FullName = mcp.ToolFullName(tool.Server, tool.Name)
		}
		if tool.FullName == "" {
			continue
		}
		set.allByName[tool.FullName] = tool

		server, configuredMCP := cfg.Servers[tool.Server]
		if isBuiltinChatDockTool(tool.FullName) || !configuredMCP || server.ExposureForTool(tool.Name, tool.FullName) == mcp.ToolExposureDirect {
			set.addVisible(tool)
			continue
		}
		onDemandTools = append(onDemandTools, tool)
	}
	set.onDemand = newToolCatalog(onDemandTools)
	if len(onDemandTools) > 0 {
		set.addVisible(builtinToolSearchTool())
	}
	return set
}

func (s *conversationToolSet) addVisible(tool mcp.MCPTool) {
	if tool.FullName == "" || s.visibleNames[tool.FullName] {
		return
	}
	s.visible = append(s.visible, tool)
	s.visibleNames[tool.FullName] = true
}

func (s *conversationToolSet) expose(tools []mcp.MCPTool) []string {
	loaded := make([]string, 0, len(tools))
	for _, tool := range tools {
		if s.visibleNames[tool.FullName] {
			continue
		}
		s.addVisible(tool)
		loaded = append(loaded, tool.FullName)
	}
	return loaded
}

func (s *conversationToolSet) tools() []mcp.MCPTool {
	return append([]mcp.MCPTool(nil), s.visible...)
}

func (s *conversationToolSet) visibleTool(name string) (mcp.MCPTool, bool) {
	if !s.visibleNames[name] || name == builtinToolSearchTools {
		return mcp.MCPTool{}, false
	}
	tool, ok := s.allByName[name]
	return tool, ok
}

func (a *App) loadConversationTools(ctx context.Context, workspaceID string, emit func(string, any) error) ([]mcp.MCPTool, mcp.MCPConfig, bool, error) {
	allTools := builtinChatDockTools()
	mcpConfig, err := a.activeMCPConfig(workspaceID)
	if err != nil || len(mcpConfig.Servers) == 0 {
		return allTools, mcpConfig, false, nil
	}

	mcpTools, err := a.mcpClient.ListTools(ctx, mcpConfig)
	if err != nil {
		if emit != nil {
			if emitErr := emit("tool_setup_error", map[string]any{"message": err.Error()}); emitErr != nil {
				return nil, mcpConfig, false, emitErr
			}
		}
		return allTools, mcpConfig, false, nil
	}
	return append(allTools, mcpTools...), mcpConfig, true, nil
}

func (a *App) callConversationTool(ctx context.Context, workspaceID string, sessionID string, mcpConfig mcp.MCPConfig, mcpReady bool, name string, args map[string]any, emit func(string, any) error) (any, error) {
	switch {
	case isBuiltinScheduledTaskTool(name):
		return a.callBuiltinScheduledTaskTool(ctx, workspaceID, name, args)
	case isBuiltinImageTool(name):
		return a.callBuiltinImageTool(ctx, name, args)
	case isBuiltinModelProviderTool(name):
		return a.callBuiltinModelProviderTool(ctx, workspaceID, name, args)
	}
	if !mcpReady {
		return nil, fmt.Errorf("MCP tool is not available: %s", name)
	}
	if mcpToolNeedsConfirmation(mcpConfig, name) {
		if err := a.requestMCPConfirmation(ctx, sessionID, name, args, emit); err != nil {
			return nil, err
		}
		return a.mcpClient.CallToolAfterConfirmation(ctx, mcpConfig, name, args)
	}
	return a.mcpClient.CallTool(ctx, mcpConfig, name, args)
}

func (a *App) callVisibleConversationTool(ctx context.Context, workspaceID string, toolSet *conversationToolSet, runRealTool func(string, map[string]any) (any, error), name string, args map[string]any) (any, error) {
	if name == builtinToolSearchTools {
		result, matches := searchToolCatalogWithMatches(ctx, a, workspaceID, toolSet.onDemand, args)
		result["loaded_tools"] = toolSet.expose(matches)
		return result, nil
	}
	tool, ok := toolSet.visibleTool(name)
	if !ok {
		return nil, fmt.Errorf("tool is not exposed in this conversation: %s", name)
	}
	if err := validateToolArguments(tool.InputSchema, args); err != nil {
		return nil, err
	}
	return runRealTool(name, args)
}

func (a *App) consumeChatJobGuidance(jobID string, emit func(string, any) error) ([]map[string]any, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, nil
	}
	items := a.drainChatJobGuidance(jobID)
	messages := make([]map[string]any, 0, len(items))
	for _, item := range items {
		content := "用户在你生成过程中追加了引导，请在当前任务和已完成工具结果基础上调整后续回答，不要丢弃已有工具结果。\n\n" + item.Message
		messages = append(messages, map[string]any{"role": "user", "content": content})
		if emit != nil {
			if err := emit("guidance_injected", item); err != nil {
				return messages, err
			}
		}
	}
	return messages, nil
}
