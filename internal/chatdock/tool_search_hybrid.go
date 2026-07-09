package chatdock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
)

type hybridToolMatch struct {
	tool        mcp.MCPTool
	keyword     int
	semantic    float64
	semanticHit bool
	pinned      bool
}

func (a *App) searchToolCatalog(ctx context.Context, workspaceID string, catalog toolCatalog, args map[string]any) map[string]any {
	return searchToolCatalogWithApp(ctx, a, workspaceID, catalog, args)
}

func searchToolCatalogWithApp(ctx context.Context, app *App, workspaceID string, catalog toolCatalog, args map[string]any) map[string]any {
	query := strings.TrimSpace(stringArg(args, "query"))
	limit := intArgWithDefault(args, "limit", 8, 1, 20)
	matches := hybridToolMatches(ctx, app, workspaceID, catalog, query, limit)
	items := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		item := map[string]any{"name": match.tool.FullName, "server": match.tool.Server, "title": firstNonEmpty(match.tool.Title, match.tool.Name), "description": compactToolDescription(match.tool.Description)}
		if match.pinned && match.keyword == 0 && !match.semanticHit {
			item["match_reason"] = "基础工具固定候选"
		} else if match.pinned {
			item["match_reason"] = "基础工具固定候选 + 搜索匹配"
		} else if match.semanticHit {
			item["match_reason"] = "关键词 + M3 向量混合匹配"
		} else if match.keyword > 0 {
			item["match_reason"] = "关键词匹配"
		}
		items = append(items, item)
	}
	return map[string]any{"query": query, "count": len(items), "tools": items, "search_mode": app.toolSearchMode(workspaceID), "next": "调用 chatdock_tools_describe，传入候选工具的 name，获取参数 schema；然后用 chatdock_tool_execute 执行。"}
}

func hybridToolMatches(ctx context.Context, app *App, workspaceID string, catalog toolCatalog, query string, limit int) []hybridToolMatch {
	keyword := keywordToolScores(catalog, query)
	semantic := app.semanticToolScores(ctx, workspaceID, catalog, query)
	matches := make([]hybridToolMatch, 0, len(catalog.tools))
	for _, tool := range catalog.tools {
		match := hybridToolMatch{tool: tool, keyword: keyword[tool.FullName]}
		if score, ok := semantic[tool.FullName]; ok {
			match.semantic = score
			match.semanticHit = true
		}
		if strings.TrimSpace(query) == "" || match.keyword > 0 || match.semanticHit {
			matches = append(matches, match)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left := float64(matches[i].keyword) + matches[i].semantic*6
		right := float64(matches[j].keyword) + matches[j].semantic*6
		if left == right {
			return matches[i].tool.FullName < matches[j].tool.FullName
		}
		return left > right
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return appendPinnedToolMatches(catalog, matches)
}

func appendPinnedToolMatches(catalog toolCatalog, matches []hybridToolMatch) []hybridToolMatch {
	existing := map[string]int{}
	for index, match := range matches {
		existing[match.tool.FullName] = index
	}
	for _, candidate := range pinnedToolSearchCandidates {
		tool, ok := findPinnedCatalogTool(catalog, candidate)
		if !ok {
			continue
		}
		if index, ok := existing[tool.FullName]; ok {
			matches[index].pinned = true
			continue
		}
		matches = append(matches, hybridToolMatch{tool: tool, pinned: true})
		existing[tool.FullName] = len(matches) - 1
	}
	return matches
}

func findPinnedCatalogTool(catalog toolCatalog, candidate string) (mcp.MCPTool, bool) {
	for _, tool := range catalog.tools {
		if toolNameMatches(tool.FullName, tool.Name, candidate) {
			return tool, true
		}
	}
	return mcp.MCPTool{}, false
}

var pinnedToolSearchCandidates = []string{"exec_command", "skill_read", "skill_run", "workflow_template_manage", "task_manage"}

func keywordToolScores(catalog toolCatalog, query string) map[string]int {
	query = strings.ToLower(strings.TrimSpace(query))
	scores := map[string]int{}
	if query == "" {
		for _, tool := range catalog.tools {
			scores[tool.FullName] = 1
		}
		return scores
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		terms = []string{query}
	}
	for _, tool := range catalog.tools {
		text := strings.ToLower(toolSearchText(tool))
		score := 0
		for _, term := range terms {
			if strings.Contains(strings.ToLower(tool.FullName), term) || strings.Contains(strings.ToLower(tool.Name), term) {
				score += 5
			}
			if strings.Contains(strings.ToLower(tool.Title), term) {
				score += 3
			}
			if strings.Contains(text, term) {
				score++
			}
		}
		if score > 0 {
			scores[tool.FullName] = score
		}
	}
	return scores
}

func (a *App) semanticToolScores(ctx context.Context, workspaceID string, catalog toolCatalog, query string) map[string]float64 {
	if a == nil || a.store == nil || a.client == nil {
		return nil
	}
	cfg := a.embeddingConfig(workspaceID)
	if strings.TrimSpace(query) == "" || strings.TrimSpace(cfg.EmbeddingBaseURL) == "" {
		return nil
	}
	records, err := a.availableToolEmbeddingIndex(workspaceID, catalog, cfg.EmbeddingModel)
	if err != nil || len(records) == 0 {
		return nil
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer queryCancel()
	queryVector, ok := a.cachedQueryEmbedding(queryCtx, cfg, query)
	if !ok {
		return nil
	}
	scores := map[string]float64{}
	for name, record := range records {
		if score := cosineSimilarity(queryVector, record.Embedding); score > 0.18 {
			scores[name] = score
		}
	}
	return scores
}

func (a *App) cachedQueryEmbedding(ctx context.Context, cfg model.ModelConfig, query string) ([]float64, bool) {
	key := strings.TrimSpace(cfg.EmbeddingModel) + "\x00" + normalizeEmbeddingQuery(query)
	a.embeddingMu.Lock()
	if vector, ok := a.embeddingMemo[key]; ok && len(vector) > 0 {
		a.embeddingMu.Unlock()
		return append([]float64(nil), vector...), true
	}
	a.embeddingMu.Unlock()
	vectors, err := a.client.Embed(ctx, cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, []string{query})
	if err != nil || len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, false
	}
	a.embeddingMu.Lock()
	if len(a.embeddingMemo) > 256 {
		for k := range a.embeddingMemo {
			delete(a.embeddingMemo, k)
			break
		}
	}
	a.embeddingMemo[key] = append([]float64(nil), vectors[0]...)
	a.embeddingMu.Unlock()
	return vectors[0], true
}

func normalizeEmbeddingQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), " "))
}

func (a *App) clearQueryEmbeddingCache() {
	a.embeddingMu.Lock()
	a.embeddingMemo = make(map[string][]float64)
	a.embeddingMu.Unlock()
}

func (a *App) ensureToolEmbeddingIndex(ctx context.Context, workspaceID string, catalog toolCatalog, model string) (map[string]storepkg.ToolEmbeddingRecord, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "BAAI/bge-m3"
	}
	existing, err := a.store.ToolEmbeddings(workspaceID, model)
	if err != nil {
		return nil, err
	}
	return a.saveMissingToolEmbeddings(ctx, workspaceID, model, existing, catalog)
}

func (a *App) availableToolEmbeddingIndex(workspaceID string, catalog toolCatalog, model string) (map[string]storepkg.ToolEmbeddingRecord, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "BAAI/bge-m3"
	}
	existing, err := a.store.ToolEmbeddings(workspaceID, model)
	if err != nil {
		return nil, err
	}
	records := make(map[string]storepkg.ToolEmbeddingRecord, len(existing))
	for _, tool := range catalog.tools {
		record, ok := existing[tool.FullName]
		if !ok || record.SourceHash != toolSourceHash(tool) || len(record.Embedding) == 0 {
			continue
		}
		records[tool.FullName] = record
	}
	return records, nil
}

func (a *App) saveMissingToolEmbeddings(ctx context.Context, workspaceID string, model string, existing map[string]storepkg.ToolEmbeddingRecord, catalog toolCatalog) (map[string]storepkg.ToolEmbeddingRecord, error) {
	cfg := a.embeddingConfig(workspaceID)
	inputs := make([]string, 0)
	tools := make([]mcp.MCPTool, 0)
	for _, tool := range catalog.tools {
		if record, ok := existing[tool.FullName]; ok && record.SourceHash == toolSourceHash(tool) && len(record.Embedding) > 0 {
			continue
		}
		tools = append(tools, tool)
		inputs = append(inputs, toolSearchText(tool))
	}
	for start := 0; start < len(inputs); start += 16 {
		end := start + 16
		if end > len(inputs) {
			end = len(inputs)
		}
		vectors, err := a.client.Embed(ctx, cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, model, inputs[start:end])
		if err != nil {
			return existing, err
		}
		for i, vector := range vectors {
			tool := tools[start+i]
			record := storepkg.ToolEmbeddingRecord{FullName: tool.FullName, SourceHash: toolSourceHash(tool), EmbeddingModel: model, Embedding: vector}
			if err := a.store.SaveToolEmbedding(workspaceID, record); err != nil {
				return existing, err
			}
			existing[tool.FullName] = record
		}
	}
	return existing, nil
}

func toolSearchText(tool mcp.MCPTool) string {
	rawSchema, _ := json.Marshal(tool.InputSchema)
	return strings.Join([]string{tool.FullName, tool.Server, tool.Name, tool.Title, tool.Description, pinnedToolSearchAliases(tool), string(rawSchema)}, "\n")
}

func pinnedToolSearchAliases(tool mcp.MCPTool) string {
	switch {
	case toolNameMatches(tool.FullName, tool.Name, "exec_command"):
		return "命令 执行命令 shell terminal 终端 bash zsh command exec_command 运行命令 查看状态 git docker go test"
	case toolNameMatches(tool.FullName, tool.Name, "skill_read"):
		return "Skill 技能 skill 工具 技能发现 只读 inspect list 查看 Skill Runtime"
	case toolNameMatches(tool.FullName, tool.Name, "skill_run"):
		return "Skill 技能 skill 工具 执行 operation run Skill Runtime 调用技能"
	case toolNameMatches(tool.FullName, tool.Name, "skill_package"):
		return "Skill 技能 skill 包 生命周期 validate install rollback 安装 校验 回滚 Skill Runtime"
	case toolNameMatches(tool.FullName, tool.Name, "skill_env_manage"):
		return "Skill 技能 skill 环境变量 env registry secret plain verify 配置 Skill Runtime"
	case toolNameMatches(tool.FullName, tool.Name, "workflow_template_manage"):
		return "任务模板 workflow template match workflow_template_manage 模板匹配 多步骤流程 部署 排障 开发"
	case toolNameMatches(tool.FullName, tool.Name, "task_manage"):
		return "可恢复任务 task_manage create resume block final_review complete_after_review 多步骤流程 部署 排障 开发"
	default:
		return ""
	}
}

func toolSourceHash(tool mcp.MCPTool) string {
	sum := sha256.Sum256([]byte(toolSearchText(tool)))
	return hex.EncodeToString(sum[:])
}

func cosineSimilarity(a []float64, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, left, right float64
	for i := range a {
		dot += a[i] * b[i]
		left += a[i] * a[i]
		right += b[i] * b[i]
	}
	if left == 0 || right == 0 {
		return 0
	}
	return dot / (math.Sqrt(left) * math.Sqrt(right))
}

func (a *App) toolSearchMode(workspaceID string) string {
	if a == nil || strings.TrimSpace(a.embeddingConfig(workspaceID).EmbeddingBaseURL) == "" {
		return "keyword"
	}
	return "hybrid_m3"
}

func (a *App) embeddingConfig(workspaceID string) model.ModelConfig {
	if a == nil || a.store == nil {
		cfg := model.ModelConfig{}
		if a != nil {
			cfg.EmbeddingBaseURL = a.cfg.EmbeddingBaseURL
			cfg.EmbeddingAPIKey = a.cfg.EmbeddingAPIKey
			cfg.EmbeddingModel = a.cfg.EmbeddingModel
		}
		return model.NormalizeModelConfig(cfg)
	}
	cfg := a.store.GetModelConfig(workspaceID)
	if strings.TrimSpace(cfg.EmbeddingBaseURL) == "" && strings.TrimSpace(a.cfg.EmbeddingBaseURL) != "" {
		cfg.EmbeddingBaseURL = a.cfg.EmbeddingBaseURL
		cfg.EmbeddingAPIKey = a.cfg.EmbeddingAPIKey
		cfg.EmbeddingModel = a.cfg.EmbeddingModel
	}
	return model.NormalizeModelConfig(cfg)
}
