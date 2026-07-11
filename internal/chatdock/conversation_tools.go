package chatdock

import (
	"context"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

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

func (a *App) callDiscoveryTool(ctx context.Context, workspaceID string, catalog toolCatalog, describedTools map[string]bool, runRealTool func(string, map[string]any) (any, error), name string, args map[string]any) (any, error) {
	switch name {
	case builtinToolSearchTools:
		return a.searchToolCatalog(ctx, workspaceID, catalog, args), nil
	case builtinToolDescribeTools:
		result, names := catalog.Describe(args)
		for _, toolName := range names {
			describedTools[toolName] = true
		}
		return result, nil
	case builtinToolExecuteDiscovered:
		return executeDiscoveredTool(catalog, describedTools, runRealTool, args)
	default:
		return nil, fmt.Errorf("unsupported discovery tool: %s; use %s after loading its schema with %s", name, builtinToolExecuteDiscovered, builtinToolDescribeTools)
	}
}

func executeDiscoveredTool(catalog toolCatalog, describedTools map[string]bool, runRealTool func(string, map[string]any) (any, error), args map[string]any) (any, error) {
	if parseErr := stringArg(args, "_parse_error"); parseErr != "" {
		raw := truncateRunes(strings.TrimSpace(stringArg(args, "_raw_arguments")), 360)
		return nil, fmt.Errorf("chatdock_tool_execute 参数 JSON 解析失败：%s。正确格式是 {\"name\":\"工具 full_name\",\"arguments\":{...}}，name 必须是顶层字段；不要把超长命令/脚本写坏 JSON。原始参数片段：%s", parseErr, raw)
	}
	target, err := requiredStringArg(args, "name")
	if err != nil {
		return nil, fmt.Errorf("chatdock_tool_execute 缺少顶层 name。正确格式是 {\"name\":\"DockMini__exec_command\",\"arguments\":{\"cmd\":\"...\"}}；name 不是目标工具 arguments 里的字段")
	}
	targetTool, ok := catalog.Get(target)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", target)
	}
	if !describedTools[target] {
		return nil, fmt.Errorf("tool schema not loaded: call %s with names=[%q] before executing it", builtinToolDescribeTools, target)
	}
	targetArgs, ok := args["arguments"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("chatdock_tool_execute 缺少顶层 arguments 对象。正确格式是 {\"name\":%q,\"arguments\":{...}}", target)
	}
	if err := validateToolArguments(targetTool.InputSchema, targetArgs); err != nil {
		return nil, err
	}
	result, err := runRealTool(target, targetArgs)
	return map[string]any{"tool": target, "result": result}, err
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
