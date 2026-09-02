package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"chatdock/internal/model"
)

const (
	contextSafetyRatio       = 0.10
	contextCompressionHigh   = 0.30
	contextCompressionTarget = 0.15
	contextSummaryRatio      = 0.40
	minimumSafetyTokens      = 2 * 1024
)

var ErrContextBudgetExceeded = errors.New("context budget exceeded")

type ContextBudget struct {
	MaxContextTokens          int  `json:"max_context_tokens"`
	OutputReserveTokens       int  `json:"output_reserve_tokens"`
	SafetyMarginTokens        int  `json:"safety_margin_tokens"`
	AvailableInputTokens      int  `json:"available_input_tokens"`
	FixedOverheadTokens       int  `json:"fixed_overhead_tokens"`
	ToolOverheadTokens        int  `json:"tool_overhead_tokens"`
	HistoryTokens             int  `json:"history_tokens"`
	CompressibleHistoryTokens int  `json:"compressible_history_tokens"`
	CompressionTriggerTokens  int  `json:"compression_trigger_tokens"`
	CompressionTargetTokens   int  `json:"compression_target_tokens"`
	TotalTokens               int  `json:"total_tokens"`
	NextCompression           bool `json:"next_compression"`
	LimitsEstimated           bool `json:"limits_estimated"`
}

type ContextPreparation struct {
	Messages           []ContextMessage
	Budget             ContextBudget
	Summary            string
	CutoffMessageID    string
	CutoffMessageIndex int
	Compressed         bool
}

type ContextCheckpoint struct {
	Summary         string
	CutoffMessageID string
}

func ContextBudgetForConfig(cfg model.ModelConfig) ContextBudget {
	cfg = model.NormalizeModelConfig(cfg)
	safety := int(float64(cfg.ContextWindowTokens) * contextSafetyRatio)
	if safety < minimumSafetyTokens {
		safety = minimumSafetyTokens
	}
	available := cfg.ContextWindowTokens - cfg.OutputReserveTokens - safety
	if available < 1 {
		available = 1
	}
	return ContextBudget{
		MaxContextTokens:         cfg.ContextWindowTokens,
		OutputReserveTokens:      cfg.OutputReserveTokens,
		SafetyMarginTokens:       safety,
		AvailableInputTokens:     available,
		CompressionTriggerTokens: int(float64(available) * contextCompressionHigh),
		CompressionTargetTokens:  int(float64(available) * contextCompressionTarget),
		LimitsEstimated:          cfg.ContextLimitsEstimated,
	}
}

func EstimateTokens(content string) int {
	han, other := 0, 0
	for _, r := range content {
		if unicode.Is(unicode.Han, r) {
			han++
		} else if !unicode.IsSpace(r) {
			other++
		}
	}
	return han + (other+3)/4
}

func EstimateAnyTokens(value any) int {
	if value == nil {
		return 0
	}
	if text, ok := value.(string); ok {
		return EstimateTokens(text)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return EstimateTokens(string(raw))
}

func EstimateToolsTokens(tools []map[string]any) int {
	tokens := 0
	for _, tool := range tools {
		tokens += EstimateAnyTokens(tool) + 8
	}
	return tokens
}

func EstimateContextMessageTokens(message ContextMessage) int {
	tokens := EstimateTokens(message.Role) + EstimateTokens(message.Content) + 4
	if message.IncludeToolHistory {
		tokens += EstimateTokens(completedToolTrace(message.Events))
	}
	return tokens
}

// PrepareChatContext 用 Token 水位决定是否压缩。消息数量只作为数据，不参与任何触发判断。
func PrepareChatContext(cfg model.ModelConfig, history []model.Message) (ContextPreparation, error) {
	return PrepareChatContextWithCheckpoint(cfg, history, nil)
}

func PrepareChatContextWithCheckpoint(cfg model.ModelConfig, history []model.Message, checkpoint *ContextCheckpoint) (ContextPreparation, error) {
	history = historyAfterCheckpoint(history, checkpoint)
	return prepareChatContext(cfg, history)
}

func prepareChatContext(cfg model.ModelConfig, history []model.Message) (ContextPreparation, error) {
	cfg = model.NormalizeModelConfig(cfg)
	valid := validChatHistory(history)
	historySystems, conversation := splitHistorySystemMessages(valid)
	base := make([]ContextMessage, 0, len(historySystems)+len(conversation)+1)
	if systemPrompt := buildSystemPrompt(cfg); systemPrompt != "" {
		base = append(base, ContextMessage{Role: "system", Content: systemPrompt})
	}
	for _, item := range historySystems {
		base = append(base, ContextMessage{Role: item.Role, Content: item.Content, SourceMessageID: item.ID, SourceMessageIndex: -1, ModelAttachments: item.ModelAttachments, Events: item.Events})
	}

	budget := ContextBudgetForConfig(cfg)
	for _, item := range base {
		budget.FixedOverheadTokens += EstimateContextMessageTokens(item)
	}
	toolHistoryIndexes := historicalToolMessageIndexSet(conversation)
	conversationTokens := make([]int, len(conversation))
	for index, item := range conversation {
		conversationTokens[index] = EstimateContextMessageTokens(ContextMessage{Role: item.Role, Content: item.Content, SourceMessageID: item.ID, SourceMessageIndex: index, ModelAttachments: item.ModelAttachments, Events: item.Events, IncludeToolHistory: toolHistoryIndexes[index]})
		budget.HistoryTokens += conversationTokens[index]
	}
	start := recentConversationStart(conversation)
	for _, tokens := range conversationTokens[:start] {
		budget.CompressibleHistoryTokens += tokens
	}
	budget.TotalTokens = budget.FixedOverheadTokens + budget.HistoryTokens
	budget.NextCompression = budget.CompressibleHistoryTokens >= budget.CompressionTriggerTokens || budget.TotalTokens > budget.AvailableInputTokens

	if budget.FixedOverheadTokens+sumInts(conversationTokens[start:]) > budget.AvailableInputTokens {
		return ContextPreparation{Budget: budget, CutoffMessageIndex: -1}, contextBudgetError("固定提示词和最近一轮消息已超过可用上下文")
	}
	if !budget.NextCompression || start == 0 {
		return ContextPreparation{Messages: append(base, conversationMessages(conversation, 0, toolHistoryIndexes)...), Budget: budget, CutoffMessageIndex: -1}, nil
	}

	maxSummary := int(float64(budget.CompressionTargetTokens) * contextSummaryRatio)
	remaining := budget.AvailableInputTokens - budget.FixedOverheadTokens - sumInts(conversationTokens[start:])
	if remaining < maxSummary {
		maxSummary = remaining
	}
	summary := summarizeHistoryWithinBudget(conversation[:start], maxSummary)
	messages := append([]ContextMessage(nil), base...)
	if summary != "" {
		messages = append(messages, ContextMessage{Role: "system", Content: summary})
	}
	messages = append(messages, conversationMessages(conversation[start:], start, toolHistoryIndexes)...)
	budget.HistoryTokens = sumContextMessageTokens(messages[len(base):])
	budget.TotalTokens = budget.FixedOverheadTokens + budget.HistoryTokens
	if budget.TotalTokens > budget.AvailableInputTokens {
		return ContextPreparation{Messages: messages, Budget: budget, Summary: summary, CutoffMessageIndex: -1, Compressed: true}, contextBudgetError("压缩后的会话历史仍超过可用上下文")
	}
	cutoffIndex := -1
	if start > 0 {
		cutoffIndex = messageIndexByID(valid, conversation[start-1].ID)
	}
	return ContextPreparation{Messages: messages, Budget: budget, Summary: summary, CutoffMessageID: conversation[start-1].ID, CutoffMessageIndex: cutoffIndex, Compressed: true}, nil
}

func historyAfterCheckpoint(history []model.Message, checkpoint *ContextCheckpoint) []model.Message {
	if checkpoint == nil || strings.TrimSpace(checkpoint.Summary) == "" || strings.TrimSpace(checkpoint.CutoffMessageID) == "" {
		return history
	}
	cutoff := -1
	for index, item := range history {
		if item.ID == checkpoint.CutoffMessageID {
			cutoff = index
			break
		}
	}
	if cutoff < 0 {
		return history
	}
	// 系统消息会在构造阶段统一提升到前缀；摘要放在这里即可保持与历史系统提示词同一顺序。
	out := make([]model.Message, 0, len(history)-cutoff+1)
	out = append(out, model.Message{Role: "system", Content: checkpoint.Summary})
	out = append(out, history[cutoff+1:]...)
	return out
}

func BuildBudgetedChatContext(cfg model.ModelConfig, history []model.Message) (ContextPreparation, error) {
	return PrepareChatContext(cfg, history)
}

func contextBudgetError(detail string) error {
	return fmt.Errorf("%w: %s", ErrContextBudgetExceeded, detail)
}

func recentConversationStart(conversation []model.Message) int {
	lastUser := -1
	for index := len(conversation) - 1; index >= 0; index-- {
		if conversation[index].Role == "user" {
			lastUser = index
			break
		}
	}
	if lastUser < 0 {
		return len(conversation)
	}
	// 当前用户消息和它前面的最近一轮完整对话必须保留原文，避免摘要遮蔽正在处理的问题。
	start := lastUser
	for index := lastUser - 1; index >= 0; index-- {
		if conversation[index].Role == "user" {
			start = index
			break
		}
	}
	return start
}

func conversationMessages(history []model.Message, sourceOffset int, toolHistoryIndexes map[int]bool) []ContextMessage {
	out := make([]ContextMessage, 0, len(history))
	for index, item := range history {
		sourceIndex := sourceOffset + index
		out = append(out, ContextMessage{Role: item.Role, Content: item.Content, SourceMessageID: item.ID, SourceMessageIndex: sourceIndex, ModelAttachments: item.ModelAttachments, Events: item.Events, IncludeToolHistory: toolHistoryIndexes[sourceIndex]})
	}
	return out
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func sumContextMessageTokens(messages []ContextMessage) int {
	total := 0
	for _, message := range messages {
		total += EstimateContextMessageTokens(message)
	}
	return total
}

func messageIndexByID(history []model.Message, id string) int {
	for index, item := range history {
		if item.ID == id {
			return index
		}
	}
	return -1
}

func summarizeHistoryWithinBudget(history []model.Message, maxTokens int) string {
	if len(history) == 0 || maxTokens <= 0 {
		return ""
	}
	header := "# 早期会话摘要\n\n以下内容由 ChatDock 本地确定性整理，用于延续早期上下文；最近一轮对话仍保留原文。\n"
	if EstimateTokens(header) > maxTokens {
		return ""
	}
	var body strings.Builder
	used := EstimateTokens(header)
	for _, item := range history {
		content := item.Content
		if strings.TrimSpace(content) == "" && hasModelImageAttachment(item.ModelAttachments) {
			content = "[图片附件]"
		}
		line := fmt.Sprintf("- %s：%s\n", contextRoleLabel(item.Role), compactContextText(content, 240))
		lineTokens := EstimateTokens(line)
		if used+lineTokens > maxTokens {
			break
		}
		body.WriteString(line)
		used += lineTokens
	}
	if body.Len() == 0 {
		return ""
	}
	return header + body.String()
}

func (p ContextPreparation) ContextMessages() []ContextMessage {
	return append([]ContextMessage(nil), p.Messages...)
}

// FitRawMessagesForContext 对包含工具结果的完整请求再做一次预算检查。
// 工具结果可能远大于普通消息，因此不能只依赖发送前的历史摘要。
func FitRawMessagesForContext(cfg model.ModelConfig, messages []map[string]any, tools []map[string]any) ([]map[string]any, ContextBudget, error) {
	return fitRawMessagesForContext(cfg, messages, tools, false)
}

func FitRawMessagesForContextAggressive(cfg model.ModelConfig, messages []map[string]any, tools []map[string]any) ([]map[string]any, ContextBudget, error) {
	return fitRawMessagesForContext(cfg, messages, tools, true)
}

func fitRawMessagesForContext(cfg model.ModelConfig, messages []map[string]any, tools []map[string]any, aggressive bool) ([]map[string]any, ContextBudget, error) {
	budget := ContextBudgetForConfig(cfg)
	staticCount := 0
	for staticCount < len(messages) && strings.TrimSpace(fmt.Sprint(messages[staticCount]["role"])) == "system" {
		budget.FixedOverheadTokens += EstimateAnyTokens(messages[staticCount]) + 4
		staticCount++
	}
	budget.ToolOverheadTokens = EstimateToolsTokens(tools)
	budget.FixedOverheadTokens += budget.ToolOverheadTokens
	nonSystem := messages[staticCount:]
	messageTokens := make([]int, len(nonSystem))
	for index, message := range nonSystem {
		messageTokens[index] = EstimateAnyTokens(message) + 4
		budget.HistoryTokens += messageTokens[index]
	}
	start := rawRecentStart(nonSystem)
	budget.CompressibleHistoryTokens = sumInts(messageTokens[:start])
	budget.TotalTokens = budget.FixedOverheadTokens + budget.HistoryTokens
	budget.NextCompression = budget.CompressibleHistoryTokens >= budget.CompressionTriggerTokens || budget.TotalTokens > budget.AvailableInputTokens
	if budget.FixedOverheadTokens+sumInts(messageTokens[start:]) > budget.AvailableInputTokens {
		// 当前 Active Turn 仍需保留 assistant.tool_calls 与每个 tool_call_id 的配对；
		// 这里只从最早的已配对 tool.content 开始折叠，不改 tool_calls 的 ID、名称或 arguments。
		nonToolTokens := 0
		for _, message := range nonSystem[start:] {
			if strings.TrimSpace(fmt.Sprint(message["role"])) == "tool" {
				continue
			}
			nonToolTokens += EstimateAnyTokens(message) + 4
		}
		maxToolTokens := budget.AvailableInputTokens - budget.FixedOverheadTokens - nonToolTokens
		if maxToolTokens > 0 {
			rebalanceToolMessagesToTokens(messages, staticCount+start, maxToolTokens)
			messageTokens = make([]int, len(nonSystem))
			for index, message := range nonSystem {
				messageTokens[index] = EstimateAnyTokens(message) + 4
			}
			budget.HistoryTokens = sumInts(messageTokens)
			budget.TotalTokens = budget.FixedOverheadTokens + budget.HistoryTokens
		}
	}
	if budget.FixedOverheadTokens+sumInts(messageTokens[start:]) > budget.AvailableInputTokens {
		return nil, budget, contextBudgetError("固定提示词、工具 schema 和最近一轮消息已超过可用上下文")
	}
	if !aggressive && !budget.NextCompression {
		return messages, budget, nil
	}
	if start == 0 {
		return messages, budget, nil
	}
	remaining := budget.AvailableInputTokens - budget.FixedOverheadTokens - sumInts(messageTokens[start:])
	compressionTarget := budget.CompressionTargetTokens
	if aggressive {
		// 供应商已经拒绝过一次时再收紧一档，但仍保留当前用户消息和最近一轮原文。
		compressionTarget = maxInt(1024, compressionTarget/2)
	}
	maxSummary := int(float64(compressionTarget) * contextSummaryRatio)
	if remaining < maxSummary {
		maxSummary = remaining
	}
	olderSummary := summarizeRawHistoryWithinBudget(nonSystem[:start], maxSummary)
	out := append([]map[string]any(nil), messages[:staticCount]...)
	if olderSummary != "" {
		out = append(out, map[string]any{"role": "system", "content": olderSummary})
	}
	out = append(out, nonSystem[start:]...)
	budget.HistoryTokens = 0
	for _, message := range out[staticCount:] {
		budget.HistoryTokens += EstimateAnyTokens(message) + 4
	}
	budget.TotalTokens = budget.FixedOverheadTokens + budget.HistoryTokens
	if budget.TotalTokens > budget.AvailableInputTokens {
		return nil, budget, contextBudgetError("压缩后的工具上下文仍超过可用上下文")
	}
	return out, budget, nil
}

func rebalanceToolMessagesToTokens(messages []map[string]any, startIndex int, maxTokens int) {
	if maxTokens <= 0 || startIndex >= len(messages) {
		return
	}
	total := 0
	indexes := make([]int, 0)
	for index := startIndex; index < len(messages); index++ {
		if strings.TrimSpace(fmt.Sprint(messages[index]["role"])) != "tool" {
			continue
		}
		total += EstimateAnyTokens(messages[index]) + 4
		indexes = append(indexes, index)
	}
	for _, index := range indexes {
		if total <= maxTokens {
			break
		}
		content, _ := messages[index]["content"].(string)
		stub := omittedToolContent(messages[index], len(content))
		if len(stub) >= len(content) {
			continue
		}
		before := EstimateAnyTokens(messages[index]) + 4
		messages[index]["content"] = stub
		total -= before - (EstimateAnyTokens(messages[index]) + 4)
	}
}

func rawRecentStart(messages []map[string]any) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if strings.TrimSpace(fmt.Sprint(messages[index]["role"])) == "user" {
			// Raw 阶段只把当前用户输入及其后的 Active Turn 视为不可压缩尾部。
			// 上一轮已经完成的工具轨迹可以安全进入历史摘要，不能再锁死整个请求。
			return index
		}
	}
	return len(messages)
}

func summarizeRawHistoryWithinBudget(history []map[string]any, maxTokens int) string {
	if len(history) == 0 || maxTokens <= 0 {
		return ""
	}
	converted := make([]model.Message, 0, len(history))
	for _, item := range history {
		converted = append(converted, model.Message{Role: strings.TrimSpace(fmt.Sprint(item["role"])), Content: strings.TrimSpace(fmt.Sprint(item["content"]))})
	}
	return summarizeHistoryWithinBudget(converted, maxTokens)
}
