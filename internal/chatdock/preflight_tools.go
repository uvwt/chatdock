package chatdock

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"chatdock/internal/chatdock/model"
)

type conversationPreflightDecision struct {
	NeedsMemory       bool
	NeedsTaskTemplate bool
	Reason            string
}

type conversationPreflightResult struct {
	Decision           conversationPreflightDecision
	MemoryTool         string
	MemoryResult       any
	MemoryError        string
	TaskTemplateTool   string
	TaskTemplateResult any
	TaskTemplateError  string
}

func decideConversationPreflight(history []model.Message) conversationPreflightDecision {
	current := latestUserText(history)
	if current == "" {
		return conversationPreflightDecision{Reason: "empty"}
	}

	currentProject := containsAnyFold(current, projectContextSignals)
	recentProject := currentProject || recentHistoryContains(history, projectContextSignals, 6)
	hardAction := hasHardAction(current)
	explainOnly := hasExplainOnlySignal(current) && !hardAction
	continuation := isContinuationOnly(current)

	if continuation {
		if recentHistoryContains(history[:max(0, len(history)-1)], operationContextSignals, 8) && recentProject {
			return conversationPreflightDecision{NeedsMemory: true, NeedsTaskTemplate: true, Reason: "continue_recent_project_operation"}
		}
		if recentProject {
			return conversationPreflightDecision{NeedsMemory: true, Reason: "continue_project_context"}
		}
		return conversationPreflightDecision{Reason: "continue_plain_context"}
	}

	if explainOnly {
		return conversationPreflightDecision{NeedsMemory: recentProject, Reason: "project_discussion_or_explanation"}
	}

	if recentProject && hardAction {
		return conversationPreflightDecision{NeedsMemory: true, NeedsTaskTemplate: true, Reason: "project_action_task"}
	}

	if hardAction && containsAnyFold(current, highRiskContextSignals) {
		return conversationPreflightDecision{NeedsMemory: true, NeedsTaskTemplate: true, Reason: "high_risk_operation"}
	}

	if recentProject {
		return conversationPreflightDecision{NeedsMemory: true, Reason: "project_context"}
	}

	return conversationPreflightDecision{Reason: "plain_chat"}
}

func (a *App) runConversationPreflight(ctx context.Context, history []model.Message, catalog toolCatalog, runTool func(string, map[string]any) (any, error), emit func(string, any) error) conversationPreflightResult {
	result := conversationPreflightResult{Decision: decideConversationPreflight(history)}
	if !result.Decision.NeedsMemory && !result.Decision.NeedsTaskTemplate {
		return result
	}
	if runTool == nil {
		return result
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	query := preflightQuery(history)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var emitMu sync.Mutex
	safeEmit := func(event string, value any) error {
		if emit == nil {
			return nil
		}
		emitMu.Lock()
		defer emitMu.Unlock()
		return emit(event, value)
	}

	if result.Decision.NeedsMemory {
		if tool, ok := findCatalogTool(catalog, []string{"memory_recall", "recall_bootstrap", "memory_search", "notes_search"}); ok {
			result.MemoryTool = tool.FullName
			wg.Add(1)
			go func() {
				defer wg.Done()
				args := preflightMemoryArgs(tool.Name, query)
				value, err := callPreflightTool(ctx, tool.FullName, args, runTool, safeEmit)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					result.MemoryError = err.Error()
					return
				}
				result.MemoryResult = value
			}()
		} else {
			result.MemoryError = "memory tool not found"
		}
	}

	if result.Decision.NeedsTaskTemplate {
		if tool, ok := findCatalogTool(catalog, []string{"task_manage"}); ok {
			result.TaskTemplateTool = tool.FullName
			wg.Add(1)
			go func() {
				defer wg.Done()
				args := map[string]any{
					"action":          "template_match",
					"task_type":       query,
					"device":          "DockMini",
					"selected_reason": "ChatDock 会话前置规则判定当前请求属于项目操作任务，需先匹配任务模板。",
				}
				value, err := callPreflightTool(ctx, tool.FullName, args, runTool, safeEmit)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					result.TaskTemplateError = err.Error()
					return
				}
				result.TaskTemplateResult = value
			}()
		} else {
			result.TaskTemplateError = "task_manage tool not found"
		}
	}

	wg.Wait()
	return result
}

func appendPreflightContext(history []model.Message, result conversationPreflightResult) []model.Message {
	content := result.ContextText()
	if strings.TrimSpace(content) == "" {
		return history
	}
	out := append([]model.Message(nil), history...)
	out = append(out, model.Message{Role: "system", Content: content, CreatedAt: time.Now()})
	return out
}

func (r conversationPreflightResult) ContextText() string {
	if !r.Decision.NeedsMemory && !r.Decision.NeedsTaskTemplate {
		return ""
	}
	lines := []string{
		"# ChatDock 会话前置上下文",
		"",
		"以下内容由 ChatDock 在模型回答前按本地规则自动检索；不是用户原文。请结合这些结果回答或执行，不要重复调用同类前置工具，除非用户明确要求更新。",
		fmt.Sprintf("- 决策：memory=%t, task_template=%t, reason=%s", r.Decision.NeedsMemory, r.Decision.NeedsTaskTemplate, r.Decision.Reason),
	}
	if r.MemoryTool != "" || r.MemoryError != "" {
		lines = append(lines, "", "## 记忆工具结果")
		if r.MemoryTool != "" {
			lines = append(lines, "- tool: "+r.MemoryTool)
		}
		if r.MemoryError != "" {
			lines = append(lines, "- error: "+r.MemoryError)
		} else {
			lines = append(lines, compactPreflightValue(r.MemoryResult, 6000))
		}
	}
	if r.TaskTemplateTool != "" || r.TaskTemplateError != "" {
		lines = append(lines, "", "## 任务模板匹配结果")
		if r.TaskTemplateTool != "" {
			lines = append(lines, "- tool: "+r.TaskTemplateTool)
		}
		if r.TaskTemplateError != "" {
			lines = append(lines, "- error: "+r.TaskTemplateError)
		} else {
			lines = append(lines, compactPreflightValue(r.TaskTemplateResult, 6000))
		}
	}
	return strings.Join(lines, "\n")
}

func callPreflightTool(ctx context.Context, name string, args map[string]any, runTool func(string, map[string]any) (any, error), emit func(string, any) error) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if emit != nil {
		if err := emit("tool_call_start", map[string]any{"tool": name, "arguments": args}); err != nil {
			return nil, err
		}
	}
	value, err := runTool(name, args)
	if emit != nil {
		payload := map[string]any{"tool": name, "result": value}
		if err != nil {
			payload["error"] = err.Error()
		}
		if emitErr := emit("tool_call_result", payload); emitErr != nil {
			return value, emitErr
		}
	}
	return value, err
}

func findCatalogTool(catalog toolCatalog, candidates []string) (catalogTool, bool) {
	for _, candidate := range candidates {
		for _, tool := range catalog.tools {
			if toolNameMatches(tool.FullName, tool.Name, candidate) {
				return catalogTool{FullName: tool.FullName, Name: tool.Name}, true
			}
		}
	}
	return catalogTool{}, false
}

type catalogTool struct {
	FullName string
	Name     string
}

func toolNameMatches(fullName, name, candidate string) bool {
	fullName = strings.ToLower(strings.TrimSpace(fullName))
	name = strings.ToLower(strings.TrimSpace(name))
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	return name == candidate || fullName == candidate || strings.HasSuffix(fullName, "__"+candidate) || strings.HasSuffix(fullName, "_"+candidate)
}

func preflightMemoryArgs(toolName string, query string) map[string]any {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "recall_bootstrap":
		return map[string]any{"project": "agentdock", "max_bytes": 8000}
	default:
		return map[string]any{"query": query, "limit": 6}
	}
}

func preflightQuery(history []model.Message) string {
	parts := make([]string, 0, 4)
	for i := len(history) - 1; i >= 0 && len(parts) < 4; i-- {
		if history[i].Role != "user" {
			continue
		}
		text := strings.TrimSpace(history[i].Content)
		if text == "" {
			continue
		}
		parts = append([]string{compactPreflightText(text, 220)}, parts...)
	}
	return strings.Join(parts, "\n")
}

func latestUserText(history []model.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return strings.TrimSpace(history[i].Content)
		}
	}
	return ""
}

func recentHistoryContains(history []model.Message, signals []string, limit int) bool {
	seen := 0
	for i := len(history) - 1; i >= 0 && seen < limit; i-- {
		if history[i].Role != "user" && history[i].Role != "assistant" {
			continue
		}
		seen++
		if containsAnyFold(history[i].Content, signals) {
			return true
		}
	}
	return false
}

func hasHardAction(text string) bool {
	if containsAnyFold(text, hardActionPhrases) {
		return true
	}
	if hasExplainOnlySignal(text) && !containsAnyFold(text, actionOverrideSignals) {
		return false
	}
	return containsAnyFold(text, operationContextSignals)
}

func hasExplainOnlySignal(text string) bool {
	return containsAnyFold(text, explainOnlySignals)
}

func isContinuationOnly(text string) bool {
	text = strings.TrimSpace(strings.Trim(text, "。！？!?.，, "))
	return containsExactFold(text, []string{"继续", "继续吧", "可以", "可以吧", "好", "好的", "行", "嗯", "开始吧", "弄吧"})
}

func containsAnyFold(text string, signals []string) bool {
	text = strings.ToLower(text)
	for _, signal := range signals {
		if strings.Contains(text, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

func containsExactFold(text string, signals []string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, signal := range signals {
		if text == strings.ToLower(signal) {
			return true
		}
	}
	return false
}

func compactPreflightValue(value any, limit int) string {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte(fmt.Sprintf("%v", value))
	}
	return compactPreflightText(string(raw), limit)
}

func compactPreflightText(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

var projectContextSignals = []string{
	"ChatDock", "ChatDoc", "AgentDock", "DockMini", "DockAir", "DockVPS", "VPS", "Docker", "compose", "反代", "部署", "数据库", "向量库", "MCP", "Skill", "工具", "定时任务", "浏览器", "截图", "服务器", "前端", "后端",
}

var operationContextSignals = []string{
	"改", "修", "查", "看一下", "排查", "部署", "提交", "推送", "测试", "优化", "添加", "删除", "调用工具", "截图", "打开浏览器", "重启", "迁移", "接入", "实现", "开发", "弄", "处理", "排障", "上线", "合并", "拉取", "构建", "验证",
}

var hardActionPhrases = []string{
	"改一下", "修一下", "查一下", "看一下", "部署一下", "提交推送", "直接改", "直接做", "开始改", "开始开发", "开始做", "做个", "截图测试", "继续改", "继续部署", "帮我弄", "处理一下", "做吧", "弄吧", "改吧", "部署吧", "测试一下",
}

var actionOverrideSignals = []string{
	"直接", "开始", "一下", "帮我", "替我", "给我", "现在", "马上", "继续", "提交推送", "打开浏览器", "截图测试", "测试一下", "部署一下", "改一下", "修一下", "查一下", "看一下", "处理一下", "做个",
}

var explainOnlySignals = []string{
	"是什么", "为什么", "原理", "建议吗", "怎么理解", "区别", "会不会", "能不能", "是不是", "应该怎么", "可以吗", "吗", "？", "?",
}

var highRiskContextSignals = []string{
	"部署", "服务器", "VPS", "Docker", "数据库", "反代", "证书", "端口", "删除", "迁移", "重启", "compose", "线上", "生产",
}
