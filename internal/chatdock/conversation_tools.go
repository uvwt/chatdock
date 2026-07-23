package chatdock

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"chatdock/internal/chatdock/mcp"
)

type conversationToolSet struct {
	visible           []mcp.MCPTool
	visibleNames      map[string]bool
	allByName         map[string]mcp.MCPTool
	onDemand          toolCatalog
	resources         map[string]*conversationToolResource
	resourceOrder     []string
	mcpConfig         mcp.MCPConfig
	loadResourceTools conversationToolLoader
}

func newConversationToolSet(allTools []mcp.MCPTool, cfg mcp.MCPConfig) *conversationToolSet {
	set := &conversationToolSet{
		visible:       make([]mcp.MCPTool, 0, len(allTools)),
		visibleNames:  map[string]bool{},
		allByName:     map[string]mcp.MCPTool{},
		onDemand:      newToolCatalog(nil),
		resources:     map[string]*conversationToolResource{},
		resourceOrder: make([]string, 0, len(cfg.Servers)+1),
		mcpConfig:     cfg,
	}

	hasBuiltinTools := false
	for _, tool := range allTools {
		if isBuiltinChatDockTool(tool.FullName) || tool.Server == builtinToolServerDiscovery {
			hasBuiltinTools = true
			break
		}
	}
	if hasBuiltinTools {
		set.resources[builtinToolResourceID] = newBuiltinToolResource(cfg.BuiltinTools.ToolExposure)
		set.resourceOrder = append(set.resourceOrder, builtinToolResourceID)
	}

	serverNames := make([]string, 0, len(cfg.Servers))
	for serverName := range cfg.Servers {
		serverNames = append(serverNames, serverName)
	}
	sort.Strings(serverNames)
	for _, serverName := range serverNames {
		set.resources[serverName] = newMCPToolResource(serverName, cfg.Servers[serverName])
		set.resourceOrder = append(set.resourceOrder, serverName)
	}

	for _, tool := range allTools {
		set.registerLoadedTool(tool)
	}
	for _, resource := range set.resources {
		if len(resource.toolNames) == 0 {
			continue
		}
		resource.info.Loaded = true
		resource.info.Status = "ready"
		resource.info.ToolCount = len(resource.toolNames)
		resource.info.ToolCountKnown = true
	}
	set.ensureDiscoveryVisibility()
	return set
}

func (s *conversationToolSet) registerLoadedTool(tool mcp.MCPTool) {
	if strings.TrimSpace(tool.FullName) == "" {
		tool.FullName = mcp.ToolFullName(tool.Server, tool.Name)
	}
	if tool.FullName == "" || isBuiltinToolDiscoveryTool(tool.FullName) {
		return
	}
	if _, exists := s.allByName[tool.FullName]; exists {
		return
	}

	resourceID := tool.Server
	exposure := mcp.ToolExposureDirect
	if isBuiltinChatDockTool(tool.FullName) {
		resourceID = builtinToolResourceID
		if s.resources[resourceID] == nil {
			s.resources[resourceID] = newBuiltinToolResource(s.mcpConfig.BuiltinTools.ToolExposure)
			s.resourceOrder = append([]string{resourceID}, s.resourceOrder...)
		}
		exposure = s.mcpConfig.BuiltinTools.ExposureForTool(tool.Name, tool.FullName)
	} else if server, configured := s.mcpConfig.Servers[resourceID]; configured {
		exposure = server.ExposureForTool(tool.Name, tool.FullName)
	} else if s.resources[resourceID] == nil {
		s.resources[resourceID] = &conversationToolResource{info: toolResource{
			ID:          resourceID,
			Title:       resourceID,
			Description: fmt.Sprintf("工具资源 %s。", resourceID),
			Kind:        "mcp",
			Exposure:    mcp.ToolExposureDirect,
			Status:      "ready",
			Loaded:      true,
		}}
		s.resourceOrder = append(s.resourceOrder, resourceID)
	}

	s.allByName[tool.FullName] = tool
	resource := s.resources[resourceID]
	if resource != nil {
		resource.toolNames = append(resource.toolNames, tool.FullName)
	}
	if exposure == mcp.ToolExposureDirect {
		s.addVisible(tool)
		return
	}
	s.addOnDemand(tool)
}

func (s *conversationToolSet) addVisible(tool mcp.MCPTool) {
	if tool.FullName == "" || s.visibleNames[tool.FullName] {
		return
	}
	s.visible = append(s.visible, tool)
	s.visibleNames[tool.FullName] = true
}

func (s *conversationToolSet) addOnDemand(tool mcp.MCPTool) {
	if _, exists := s.onDemand.byName[tool.FullName]; exists {
		return
	}
	s.onDemand.tools = append(s.onDemand.tools, tool)
	s.onDemand.byName[tool.FullName] = tool
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
	s.ensureDiscoveryVisibility()
	return loaded
}

func (s *conversationToolSet) hasDiscovery() bool {
	if len(s.onDemand.tools) > 0 {
		return true
	}
	for _, resource := range s.resources {
		if resource.info.Status != "disabled" && !resource.info.Loaded {
			return true
		}
	}
	return false
}

func (s *conversationToolSet) ensureDiscoveryVisibility() {
	if s.hasDiscovery() {
		s.visibleNames[builtinToolSearchTools] = true
		return
	}
	delete(s.visibleNames, builtinToolSearchTools)
}

func (s *conversationToolSet) tools() []mcp.MCPTool {
	tools := append([]mcp.MCPTool(nil), s.visible...)
	if s.hasDiscovery() {
		tools = append(tools, builtinToolSearchTool(s.resourceIndex()))
	}
	return tools
}

func (s *conversationToolSet) visibleTool(name string) (mcp.MCPTool, bool) {
	if !s.visibleNames[name] || name == builtinToolSearchTools {
		return mcp.MCPTool{}, false
	}
	tool, ok := s.allByName[name]
	return tool, ok
}

func (s *conversationToolSet) loadedResourceCount() int {
	count := 0
	for _, resource := range s.resources {
		if resource.info.Loaded {
			count++
		}
	}
	return count
}

func (s *conversationToolSet) onDemandResourceCount() int {
	count := 0
	for _, resource := range s.resources {
		if resource.info.Status != "disabled" && resource.info.Exposure == mcp.ToolExposureOnDemand {
			count++
		}
	}
	return count
}

func (s *conversationToolSet) resourceErrorCount() int {
	count := 0
	for _, resource := range s.resources {
		if resource.info.Status == "error" {
			count++
		}
	}
	return count
}

func (a *App) loadConversationTools(ctx context.Context, emit func(string, any) error) (*conversationToolSet, mcp.MCPConfig, error) {
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
		set.loadResourceTools = func(loadCtx context.Context, resourceID string) ([]mcp.MCPTool, error) {
			return a.mcpClient.ListServerTools(loadCtx, mcpConfig, resourceID)
		}
		// 复用近期 tools/list 缓存，让资源索引能显示工具数量，同时不额外访问远端 MCP。
		for resourceID := range mcpConfig.Servers {
			if cachedTools, ok := a.mcpClient.CachedServerTools(mcpConfig, resourceID); ok {
				set.setLoadedResourceTools(resourceID, cachedTools, "cached")
			}
		}
	}
	// direct 资源和含 direct 单工具覆盖的资源仍需预加载；纯 on_demand 资源保持懒加载。
	for _, resourceID := range set.resourceOrder {
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

func (a *App) callConversationTool(ctx context.Context, sessionID string, mcpConfig mcp.MCPConfig, name string, args map[string]any, emit func(string, any) error) (any, error) {
	switch {
	case isBuiltinScheduledTaskTool(name):
		return a.callBuiltinScheduledTaskTool(ctx, name, args)
	case isBuiltinImageTool(name):
		return a.callBuiltinImageTool(ctx, name, args)
	case isBuiltinModelProviderTool(name):
		return a.callBuiltinModelProviderTool(ctx, name, args)
	}
	if a.mcpClient == nil {
		return nil, fmt.Errorf("MCP tool client is unavailable: %s", name)
	}
	requiresConfirmation, err := a.mcpClient.ToolRequiresConfirmation(ctx, mcpConfig, name)
	if err != nil {
		return nil, err
	}
	if requiresConfirmation {
		if err := a.requestMCPConfirmation(ctx, sessionID, name, args, emit); err != nil {
			return nil, err
		}
		return a.mcpClient.CallToolAfterConfirmation(ctx, mcpConfig, name, args)
	}
	return a.mcpClient.CallTool(ctx, mcpConfig, name, args)
}

func (a *App) callVisibleConversationTool(ctx context.Context, toolSet *conversationToolSet, runRealTool func(string, map[string]any) (any, error), name string, args map[string]any) (any, error) {
	if name == builtinToolSearchTools {
		return a.discoverConversationTools(ctx, toolSet, args)
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

func (a *App) discoverConversationTools(ctx context.Context, toolSet *conversationToolSet, args map[string]any) (map[string]any, error) {
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
				resource := toolSet.resources[id]
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
		baseResult["next"] = "该资源的真实工具已加入下一次模型请求；请直接调用目标工具。"
		return baseResult, nil
	}

	catalog := toolSet.catalogForResources(resourceIDs, resourcesProvided)
	result, matches := searchToolCatalogWithMatches(ctx, a, catalog, map[string]any{"query": query, "limit": intArgWithDefault(args, "limit", 8, 1, 20)})
	for key, value := range baseResult {
		result[key] = value
	}
	result["mode"] = "search"
	result["loaded_tools"] = toolSet.expose(matches)
	if len(matches) == 0 && len(resourceIDs) == 0 {
		result["next"] = "未能仅根据目标确定资源；请从 resources 中选择一个资源后再次搜索，或对单个资源省略 query 进行全量加载。"
	}
	return result, nil
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
