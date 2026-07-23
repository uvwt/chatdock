package chatdock

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"chatdock/internal/chatdock/mcp"
)

const (
	builtinToolResourceID          = "ChatDock"
	maxBulkResourceToolDefinitions = 64
	maxToolDiscoveryQueryRunes     = 240
	maxToolDiscoveryResourceCount  = 8
)

type toolResource struct {
	ID             string
	Title          string
	Description    string
	Kind           string
	Exposure       mcp.ToolExposure
	Status         string
	ToolCount      int
	ToolCountKnown bool
	Loaded         bool
	LastError      string
}

type conversationToolResource struct {
	info      toolResource
	toolNames []string
}

type conversationToolLoader func(context.Context, string) ([]mcp.MCPTool, error)

func newBuiltinToolResource(exposure mcp.ToolExposure) *conversationToolResource {
	if exposure == "" {
		exposure = mcp.ToolExposureDirect
	}
	return &conversationToolResource{info: toolResource{
		ID:          builtinToolResourceID,
		Title:       "ChatDock",
		Description: "ChatDock 内置的定时任务、图片处理和模型供应商能力。",
		Kind:        "builtin",
		Exposure:    exposure,
		Status:      "ready",
		Loaded:      true,
	}}
}

func newMCPToolResource(name string, server mcp.MCPServerConfig) *conversationToolResource {
	status := "lazy"
	if server.Disabled || strings.TrimSpace(server.URL) == "" {
		status = "disabled"
	}
	return &conversationToolResource{info: toolResource{
		ID:          name,
		Title:       name,
		Description: mcpResourceDescription(name, server),
		Kind:        "mcp",
		Exposure:    mcpServerDefaultExposure(server),
		Status:      status,
	}}
}

func mcpResourceDescription(name string, server mcp.MCPServerConfig) string {
	if description := compactResourceText(server.Description); description != "" {
		return description
	}
	return fmt.Sprintf("MCP 资源 %s；需要具体能力时可按资源搜索或加载该资源工具。", name)
}

func compactResourceText(value string) string {
	return compactToolDescription(strings.Join(strings.Fields(value), " "))
}

func mcpServerDefaultExposure(server mcp.MCPServerConfig) mcp.ToolExposure {
	if server.ToolExposure == "" {
		return mcp.ToolExposureOnDemand
	}
	return server.ToolExposure
}

func mcpServerNeedsInitialLoad(server mcp.MCPServerConfig) bool {
	if server.Disabled || strings.TrimSpace(server.URL) == "" {
		return false
	}
	if mcpServerDefaultExposure(server) == mcp.ToolExposureDirect {
		return true
	}
	for _, exposure := range server.ToolOverrides {
		if exposure == mcp.ToolExposureDirect {
			return true
		}
	}
	return false
}

func resourceIndexText(resources []toolResource) string {
	lines := make([]string, 0, len(resources))
	for _, resource := range resources {
		count := "工具数量待加载"
		if resource.ToolCountKnown {
			count = fmt.Sprintf("%d 个工具", resource.ToolCount)
		}
		status := resource.Status
		if status == "" {
			status = "unknown"
		}
		lines = append(lines, fmt.Sprintf("- %s：%s（%s，%s，%s）", resource.ID, resource.Description, resource.Exposure, count, status))
	}
	return strings.Join(lines, "\n")
}

func resourceIndexPayload(resources []toolResource) []map[string]any {
	items := make([]map[string]any, 0, len(resources))
	for _, resource := range resources {
		item := map[string]any{
			"id":          resource.ID,
			"title":       resource.Title,
			"description": resource.Description,
			"kind":        resource.Kind,
			"exposure":    resource.Exposure,
			"status":      resource.Status,
			"loaded":      resource.Loaded,
		}
		if resource.ToolCountKnown {
			item["tool_count"] = resource.ToolCount
		}
		if resource.LastError != "" {
			item["error"] = compactResourceText(resource.LastError)
		}
		items = append(items, item)
	}
	return items
}

func (s *conversationToolSet) resourceIndex() []toolResource {
	resources := make([]toolResource, 0, len(s.resourceOrder))
	for _, id := range s.resourceOrder {
		resource := s.resources[id]
		if resource == nil {
			continue
		}
		resources = append(resources, resource.info)
	}
	return resources
}

func (s *conversationToolSet) enabledResourceIDs() []string {
	ids := make([]string, 0, len(s.resourceOrder))
	for _, id := range s.resourceOrder {
		resource := s.resources[id]
		if resource == nil || resource.info.Status == "disabled" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (s *conversationToolSet) resolveResourceID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if _, ok := s.resources[value]; ok {
		return value, true
	}
	for id := range s.resources {
		if strings.EqualFold(id, value) {
			return id, true
		}
	}
	return "", false
}

func (s *conversationToolSet) matchResourceIDs(query string, limit int) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	type scoredResource struct {
		id    string
		score int
	}
	terms := toolDiscoveryTerms(query)
	matches := make([]scoredResource, 0, len(s.resourceOrder))
	for _, id := range s.resourceOrder {
		resource := s.resources[id]
		if resource == nil || resource.info.Status == "disabled" {
			continue
		}
		resourceID := strings.ToLower(resource.info.ID)
		title := strings.ToLower(resource.info.Title)
		description := strings.ToLower(resource.info.Description)
		score := 0
		for _, term := range terms {
			if resourceID == term {
				score += 20
			} else if strings.Contains(resourceID, term) || strings.Contains(term, resourceID) {
				score += 8
			}
			if strings.Contains(title, term) || strings.Contains(term, title) {
				score += 5
			}
			if strings.Contains(description, term) {
				score += 2
			}
		}
		if score > 0 {
			matches = append(matches, scoredResource{id: id, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].id < matches[j].id
		}
		return matches[i].score > matches[j].score
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.id)
	}
	return ids
}

func (s *conversationToolSet) loadResource(ctx context.Context, id string) (*conversationToolResource, error) {
	resource := s.resources[id]
	if resource == nil {
		return nil, fmt.Errorf("tool resource not found: %s", id)
	}
	if resource.info.Status == "disabled" {
		return resource, fmt.Errorf("tool resource is disabled: %s", id)
	}
	if resource.info.Loaded {
		return resource, nil
	}
	if s.loadResourceTools == nil {
		return resource, fmt.Errorf("tool resource loader is unavailable: %s", id)
	}
	// 只有目标资源真正被选中时才请求 tools/list；其他按需资源保持未加载。
	tools, err := s.loadResourceTools(ctx, id)
	if err != nil {
		resource.info.Status = "error"
		resource.info.LastError = compactResourceText(err.Error())
		return resource, err
	}
	s.setLoadedResourceTools(id, tools, "ready")
	return resource, nil
}

func (s *conversationToolSet) setLoadedResourceTools(id string, tools []mcp.MCPTool, status string) {
	resource := s.resources[id]
	if resource == nil {
		return
	}
	for _, tool := range tools {
		s.registerLoadedTool(tool)
	}
	resource.info.Loaded = true
	resource.info.Status = status
	resource.info.LastError = ""
	resource.info.ToolCount = len(resource.toolNames)
	resource.info.ToolCountKnown = true
	s.ensureDiscoveryVisibility()
}

func (s *conversationToolSet) catalogForResources(resourceIDs []string, restrict bool) toolCatalog {
	if !restrict {
		return s.onDemand
	}
	allowed := make(map[string]bool, len(resourceIDs))
	for _, id := range resourceIDs {
		allowed[id] = true
	}
	tools := make([]mcp.MCPTool, 0, len(s.onDemand.tools))
	for _, tool := range s.onDemand.tools {
		resourceID := tool.Server
		if isBuiltinChatDockTool(tool.FullName) {
			resourceID = builtinToolResourceID
		}
		if allowed[resourceID] {
			tools = append(tools, tool)
		}
	}
	return newToolCatalog(tools)
}

func (s *conversationToolSet) toolDefinitionsForResources(resourceIDs []string) []mcp.MCPTool {
	seen := map[string]bool{}
	tools := make([]mcp.MCPTool, 0)
	for _, id := range resourceIDs {
		resource := s.resources[id]
		if resource == nil {
			continue
		}
		for _, name := range resource.toolNames {
			if seen[name] {
				continue
			}
			tool, ok := s.allByName[name]
			if !ok {
				continue
			}
			seen[name] = true
			tools = append(tools, tool)
		}
	}
	return tools
}

func toolDiscoveryTerms(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	runes := []rune(query)
	if len(runes) > maxToolDiscoveryQueryRunes {
		runes = runes[:maxToolDiscoveryQueryRunes]
		query = string(runes)
	}
	terms := strings.Fields(query)
	seen := map[string]bool{}
	out := make([]string, 0, len(terms)+8)
	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	for _, term := range terms {
		add(term)
	}
	// 中文目标通常没有空格，补充二、三字片段提高资源和工具关键词召回。
	hasHan := false
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			hasHan = true
			break
		}
	}
	if hasHan {
		for size := 2; size <= 3; size++ {
			for start := 0; start+size <= len(runes); start++ {
				candidate := runes[start : start+size]
				allHan := true
				for _, r := range candidate {
					if !unicode.Is(unicode.Han, r) {
						allHan = false
						break
					}
				}
				if allHan {
					add(string(candidate))
				}
			}
		}
	}
	return out
}
