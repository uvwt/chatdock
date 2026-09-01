package httpapi

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"chatdock/internal/mcp"
	"chatdock/internal/toolschema"
)

type conversationToolSet struct {
	loaded              toolCatalog
	onDemand            toolCatalog
	discovered          conversationToolExposure
	exposure            conversationToolExposure
	resources           conversationToolResources
	mcpConfig           mcp.MCPConfig
	serverInstructions  []mcp.MCPServerInstruction
	workingSetSessionID string
	workingSetTurn      int
}

type conversationToolExposure struct {
	tools []mcp.MCPTool
	names map[string]bool
}

func newConversationToolExposure(capacity int) conversationToolExposure {
	return conversationToolExposure{
		tools: make([]mcp.MCPTool, 0, capacity),
		names: make(map[string]bool, capacity),
	}
}

func (e *conversationToolExposure) Add(tool mcp.MCPTool) bool {
	if tool.FullName == "" || e.names[tool.FullName] {
		return false
	}
	e.tools = append(e.tools, tool)
	e.names[tool.FullName] = true
	return true
}

func (e conversationToolExposure) Has(name string) bool {
	return e.names[strings.TrimSpace(name)]
}

func (e conversationToolExposure) Tools() []mcp.MCPTool {
	return append([]mcp.MCPTool(nil), e.tools...)
}

func newConversationToolSet(allTools []mcp.MCPTool, cfg mcp.MCPConfig) *conversationToolSet {
	set := &conversationToolSet{
		loaded:     newToolCatalog(nil),
		onDemand:   newToolCatalog(nil),
		discovered: newConversationToolExposure(len(allTools)),
		exposure:   newConversationToolExposure(len(allTools)),
		resources:  newConversationToolResources(len(cfg.Servers) + 1),
		mcpConfig:  cfg,
	}

	hasBuiltinTools := false
	for _, tool := range allTools {
		if tool.Server == builtinToolServerDiscovery {
			hasBuiltinTools = true
			break
		}
	}
	if hasBuiltinTools {
		set.resources.byID[builtinToolResourceID] = newBuiltinToolResource(cfg.BuiltinTools.ToolExposure)
		set.resources.order = append(set.resources.order, builtinToolResourceID)
	}

	serverNames := make([]string, 0, len(cfg.Servers))
	for serverName := range cfg.Servers {
		serverNames = append(serverNames, serverName)
	}
	sort.Strings(serverNames)
	for _, serverName := range serverNames {
		set.resources.byID[serverName] = newMCPToolResource(serverName, cfg.Servers[serverName])
		set.resources.order = append(set.resources.order, serverName)
	}

	for _, tool := range allTools {
		set.registerLoadedTool(tool)
	}
	for _, resource := range set.resources.byID {
		if len(resource.toolNames) == 0 {
			continue
		}
		resource.info.Loaded = true
		resource.info.Status = "ready"
		resource.info.ToolCount = len(resource.toolNames)
		resource.info.ToolCountKnown = true
	}
	return set
}

func (s *conversationToolSet) registerLoadedTool(tool mcp.MCPTool) {
	if strings.TrimSpace(tool.FullName) == "" {
		tool.FullName = mcp.ToolFullName(tool.Server, tool.Name)
	}
	if tool.FullName == "" || isBuiltinToolDiscoveryTool(tool.FullName) {
		return
	}
	if !s.loaded.Add(tool) {
		return
	}

	resourceID := tool.Server
	exposure := mcp.ToolExposureDirect
	if tool.Server == builtinToolServerDiscovery {
		resourceID = builtinToolResourceID
		if s.resources.byID[resourceID] == nil {
			s.resources.byID[resourceID] = newBuiltinToolResource(s.mcpConfig.BuiltinTools.ToolExposure)
			s.resources.order = append([]string{resourceID}, s.resources.order...)
		}
		exposure = s.mcpConfig.BuiltinTools.ExposureForTool(tool.Name, tool.FullName)
	} else if server, configured := s.mcpConfig.Servers[resourceID]; configured {
		exposure = server.ExposureForTool(tool.Name, tool.FullName)
	} else if s.resources.byID[resourceID] == nil {
		s.resources.byID[resourceID] = &conversationToolResource{info: toolResource{
			ID:          resourceID,
			Title:       resourceID,
			Description: fmt.Sprintf("工具资源 %s。", resourceID),
			Kind:        "mcp",
			Exposure:    mcp.ToolExposureDirect,
			Status:      "ready",
			Loaded:      true,
		}}
		s.resources.order = append(s.resources.order, resourceID)
	}

	resource := s.resources.byID[resourceID]
	if resource != nil {
		resource.toolNames = append(resource.toolNames, tool.FullName)
	}
	if exposure == mcp.ToolExposureDirect {
		s.exposure.Add(tool)
		return
	}
	s.onDemand.Add(tool)
}

func (s *conversationToolSet) expose(tools []mcp.MCPTool) []string {
	loaded := make([]string, 0, len(tools))
	for _, tool := range tools {
		if s.exposure.Has(tool.FullName) || s.discovered.Has(tool.FullName) {
			continue
		}
		if s.discovered.Add(tool) {
			loaded = append(loaded, tool.FullName)
		}
	}
	return loaded
}

func (s *conversationToolSet) hasDiscovery() bool {
	if len(s.onDemand.tools) > 0 {
		return true
	}
	for _, resource := range s.resources.byID {
		if resource.info.Status != "disabled" && !resource.info.Loaded {
			return true
		}
	}
	return false
}

func (s *conversationToolSet) tools() []mcp.MCPTool {
	tools := s.exposure.Tools()
	if s.hasDiscovery() {
		tools = append(tools, builtinToolSearchTool(s.resourceIndex()))
		tools = append(tools, builtinToolCallTool())
	}
	return tools
}

func (s *conversationToolSet) visibleTool(name string) (mcp.MCPTool, bool) {
	name = strings.TrimSpace(name)
	if name == builtinToolSearchTools {
		if !s.hasDiscovery() {
			return mcp.MCPTool{}, false
		}
		return builtinToolSearchTool(s.resourceIndex()), true
	}
	if name == builtinToolCall {
		if !s.hasDiscovery() {
			return mcp.MCPTool{}, false
		}
		return builtinToolCallTool(), true
	}
	if !s.exposure.Has(name) && !s.discovered.Has(name) {
		return mcp.MCPTool{}, false
	}
	return s.loaded.Get(name)
}

func (a *Server) loadConversationTools(ctx context.Context, emit func(string, any) error) (*conversationToolSet, mcp.MCPConfig, error) {
	mcpConfig, err := a.activeMCPConfig()
	if err != nil {
		set := newConversationToolSet(builtinChatDockTools(), mcp.MCPConfig{})
		if emit != nil {
			if emitErr := emit("tool_setup_error", map[string]any{"stage": "config", "message": err.Error()}); emitErr != nil {
				return nil, mcp.MCPConfig{}, emitErr
			}
		}
		return set, mcp.MCPConfig{}, nil
	}

	set := newConversationToolSet(builtinChatDockTools(), mcpConfig)
	if a.mcpClient != nil {
		set.resources.loader = func(loadCtx context.Context, resourceID string) ([]mcp.MCPTool, error) {
			return a.mcpClient.ListServerTools(loadCtx, mcpConfig, resourceID)
		}
		discoveredInstructions, discoveryErrors := a.mcpClient.ServerInstructions(ctx, mcpConfig)
		set.serverInstructions = discoveredInstructions
		for serverName, discoveryErr := range discoveryErrors {
			if resource := set.resources.byID[serverName]; resource != nil {
				resource.info.Status = "error"
				resource.info.LastError = compactResourceText(discoveryErr.Error())
			}
			if emit != nil {
				if emitErr := emit("tool_setup_error", map[string]any{"stage": "discovery", "resource": serverName, "message": discoveryErr.Error()}); emitErr != nil {
					return nil, mcpConfig, emitErr
				}
			}
		}
		// 复用近期 tools/list 缓存，让资源索引能显示工具数量，同时不额外访问远端 MCP。
		for resourceID := range mcpConfig.Servers {
			if cachedTools, ok := a.mcpClient.CachedServerTools(mcpConfig, resourceID); ok {
				set.setLoadedResourceTools(resourceID, cachedTools, "cached")
			}
		}
	}
	// direct 资源和含 direct 单工具覆盖的资源仍需预加载；纯 on_demand 资源保持懒加载。
	for _, resourceID := range set.resources.order {
		server, configured := mcpConfig.Servers[resourceID]
		if !configured || !mcpServerNeedsInitialLoad(server) {
			continue
		}
		if _, loadErr := set.loadResource(ctx, resourceID); loadErr != nil && emit != nil {
			if emitErr := emit("tool_setup_error", map[string]any{"stage": "resource", "resource": resourceID, "message": loadErr.Error()}); emitErr != nil {
				return nil, mcpConfig, emitErr
			}
		}
	}
	return set, mcpConfig, nil
}

func (a *Server) callConversationTool(ctx context.Context, sessionID string, mcpConfig mcp.MCPConfig, name string, args map[string]any, emit func(string, any) error) (any, error) {
	if registration, ok := builtinChatDockToolRegistration(name); ok {
		return registration.Handler(a, ctx, args)
	}
	if a.mcpClient == nil {
		return nil, fmt.Errorf("MCP tool client is unavailable: %s", name)
	}
	requiresConfirmation, err := a.mcpClient.ToolRequiresConfirmation(ctx, mcpConfig, name)
	if err != nil {
		return nil, err
	}
	if requiresConfirmation {
		if err := a.approvals.Request(ctx, sessionID, name, args, emit); err != nil {
			return nil, err
		}
		return a.mcpClient.CallToolAfterConfirmation(ctx, mcpConfig, name, args)
	}
	return a.mcpClient.CallTool(ctx, mcpConfig, name, args)
}

func (a *Server) callVisibleConversationTool(ctx context.Context, toolSet *conversationToolSet, runRealTool func(string, map[string]any) (any, error), name string, args map[string]any) (any, error) {
	tool, ok := toolSet.visibleTool(name)
	if !ok {
		return nil, fmt.Errorf("tool is not exposed in this conversation: %s", name)
	}
	if err := toolschema.ValidateArguments(tool.InputSchema, args); err != nil {
		return nil, err
	}
	if isBuiltinToolDiscoveryTool(tool.FullName) {
		if tool.FullName == builtinToolCall {
			return a.callDiscoveredConversationTool(toolSet, runRealTool, args)
		}
		return a.discoverConversationTools(ctx, toolSet, args)
	}
	result, err := runRealTool(name, args)
	if err == nil {
		a.rememberCalledConversationTool(toolSet.workingSetSessionID, toolSet.workingSetTurn, toolSet, tool)
	}
	return result, err
}

func (a *Server) callDiscoveredConversationTool(toolSet *conversationToolSet, runRealTool func(string, map[string]any) (any, error), args map[string]any) (any, error) {
	name := strings.TrimSpace(stringArg(args, "tool"))
	arguments, ok := args["arguments"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("arguments.arguments must be object")
	}
	tool, ok := toolSet.loaded.Get(name)
	if ok && !toolSet.discovered.Has(name) && !toolSet.exposure.Has(name) {
		ok = false
	}
	if !ok {
		return nil, fmt.Errorf("tool is not loaded in this conversation: %s", name)
	}
	if err := toolschema.ValidateArguments(tool.InputSchema, arguments); err != nil {
		return nil, err
	}
	result, err := runRealTool(name, arguments)
	if err == nil {
		a.rememberCalledConversationTool(toolSet.workingSetSessionID, toolSet.workingSetTurn, toolSet, tool)
	}
	return result, err
}

func (a *Server) discoverConversationTools(ctx context.Context, toolSet *conversationToolSet, args map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringArg(args, "query"))
	if len([]rune(query)) > maxToolDiscoveryQueryRunes {
		return nil, fmt.Errorf("query exceeds %d characters", maxToolDiscoveryQueryRunes)
	}
	requestedResources, resourcesProvided, err := optionalStringSliceArg(args, "resources")
	if err != nil {
		return nil, err
	}
	if len(requestedResources) > maxToolDiscoveryResourceCount {
		return nil, fmt.Errorf("resources exceeds %d items", maxToolDiscoveryResourceCount)
	}
	if query == "" && (!resourcesProvided || len(requestedResources) == 0) {
		return nil, fmt.Errorf("query or resources is required")
	}

	resourceIDs := make([]string, 0, len(requestedResources))
	seenResources := map[string]bool{}
	for _, requested := range requestedResources {
		resourceID, ok := toolSet.resolveResourceID(requested)
		if !ok {
			return nil, fmt.Errorf("tool resource not found: %s", requested)
		}
		if seenResources[resourceID] {
			continue
		}
		seenResources[resourceID] = true
		resourceIDs = append(resourceIDs, resourceID)
	}

	if !resourcesProvided {
		resourceIDs = toolSet.matchResourceIDs(query, 3)
		if len(resourceIDs) == 0 {
			unloaded := make([]string, 0, 1)
			for _, id := range toolSet.enabledResourceIDs() {
				resource := toolSet.resources.byID[id]
				if resource != nil && !resource.info.Loaded {
					unloaded = append(unloaded, id)
				}
			}
			if len(unloaded) == 1 {
				resourceIDs = unloaded
			}
		}
	}

	resourceErrors := map[string]string{}
	for _, resourceID := range resourceIDs {
		if _, loadErr := toolSet.loadResource(ctx, resourceID); loadErr != nil {
			resourceErrors[resourceID] = compactResourceText(loadErr.Error())
		}
	}

	baseResult := map[string]any{
		"query":             query,
		"matched_resources": append([]string(nil), resourceIDs...),
		"resources":         resourceIndexPayload(toolSet.resourceIndex()),
	}
	if len(resourceErrors) > 0 {
		baseResult["resource_errors"] = resourceErrors
	}

	if query == "" {
		// 单资源无 query 是明确的多步骤场景，允许一次暴露该资源全部真实 Schema。
		definitions := toolSet.toolDefinitionsForResources(resourceIDs)
		baseResult["mode"] = "resource_bulk"
		baseResult["resource_tool_count"] = len(definitions)
		if len(resourceIDs) > 1 && len(definitions) > maxBulkResourceToolDefinitions {
			baseResult["budget_exceeded"] = true
			baseResult["loaded_tools"] = []string{}
			baseResult["next"] = fmt.Sprintf("这些资源共有 %d 个工具，超过单次批量加载预算 %d；请缩小 resources 或提供 query。", len(definitions), maxBulkResourceToolDefinitions)
			return baseResult, nil
		}
		baseResult["loaded_tools"] = toolSet.expose(definitions)
		baseResult["tool_schemas"] = toolSchemasForConversation(definitions)
		a.rememberDiscoveredConversationTools(toolSet.workingSetSessionID, toolSet.workingSetTurn, toolSet, definitions)
		baseResult["next"] = "该资源的真实 schema 已放入本轮对话尾部；请使用 chatdock_tool_call 调用目标工具。"
		return baseResult, nil
	}

	catalog := toolSet.catalogForResources(resourceIDs, resourcesProvided)
	result, matches := searchToolCatalogWithMatches(ctx, a, catalog, map[string]any{"query": query, "limit": intArgWithDefault(args, "limit", 8, 1, 20)})
	for key, value := range baseResult {
		result[key] = value
	}
	result["mode"] = "search"
	result["loaded_tools"] = toolSet.expose(matches)
	result["tool_schemas"] = toolSchemasForConversation(matches)
	a.rememberDiscoveredConversationTools(toolSet.workingSetSessionID, toolSet.workingSetTurn, toolSet, matches)
	if len(matches) == 0 && len(resourceIDs) == 0 {
		result["next"] = "未能仅根据目标确定资源；请从 resources 中选择一个资源后再次搜索，或对单个资源省略 query 进行全量加载。"
	}
	return result, nil
}

func toolSchemasForConversation(tools []mcp.MCPTool) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := tool.FullName
		if strings.TrimSpace(name) == "" {
			name = mcp.ToolFullName(tool.Server, tool.Name)
		}
		result = append(result, map[string]any{
			"name":        name,
			"description": firstNonEmpty(tool.Description, tool.Title),
			"parameters":  tool.InputSchema,
		})
	}
	return result
}

func (a *Server) consumeChatJobGuidance(jobID string, emit func(string, any) error) ([]map[string]any, error) {
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
