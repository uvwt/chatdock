package store

import (
	"strings"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

type ContextPreviewItem struct {
	Role            string `json:"role"`
	Source          string `json:"source"`
	Chars           int    `json:"chars"`
	EstimatedTokens int    `json:"estimated_tokens"`
	ContentPreview  string `json:"content_preview"`
}

type ContextPreviewResponse struct {
	SessionID                string                  `json:"session_id"`
	ProjectID                string                  `json:"project_id,omitempty"`
	ProviderID               string                  `json:"provider_id,omitempty"`
	Model                    string                  `json:"model"`
	ContextMode              string                  `json:"context_mode"`
	RecentMessages           int                     `json:"recent_messages"`
	SummarizeOld             bool                    `json:"summarize_old"`
	MessageCount             int                     `json:"message_count"`
	ContextCount             int                     `json:"context_count"`
	TotalChars               int                     `json:"total_chars"`
	EstimatedTokens          int                     `json:"estimated_tokens"`
	MaxContextTokens         int                     `json:"max_context_tokens"`
	OutputReserveTokens      int                     `json:"output_reserve_tokens"`
	SafetyMarginTokens       int                     `json:"safety_margin_tokens"`
	AvailableInputTokens     int                     `json:"available_input_tokens"`
	FixedOverheadTokens      int                     `json:"fixed_overhead_tokens"`
	ToolOverheadTokens       int                     `json:"tool_overhead_tokens"`
	HistoryTokens            int                     `json:"history_tokens"`
	CompressionTriggerTokens int                     `json:"compression_trigger_tokens"`
	CompressionTargetTokens  int                     `json:"compression_target_tokens"`
	NextCompression          bool                    `json:"next_compression"`
	LimitsEstimated          bool                    `json:"limits_estimated"`
	Checkpoint               ContextCheckpointStatus `json:"checkpoint"`
	Items                    []ContextPreviewItem    `json:"items"`
}

func (s *Store) ContextPreview(sessionID string) (ContextPreviewResponse, error) {
	return s.contextPreview(sessionID, 0)
}

func (s *Store) ContextPreviewWithToolOverhead(sessionID string, toolOverheadTokens int) (ContextPreviewResponse, error) {
	return s.contextPreview(sessionID, toolOverheadTokens)
}

func (s *Store) contextPreview(sessionID string, toolOverheadTokens int) (ContextPreviewResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if toolOverheadTokens < 0 {
		toolOverheadTokens = 0
	}
	session, ok, err := s.sessionLocked(strings.TrimSpace(sessionID))
	if err != nil {
		return ContextPreviewResponse{}, err
	}
	if !ok {
		return ContextPreviewResponse{}, model.ErrSessionNotFound
	}
	cfg, err := s.modelConfigLocked()
	if err != nil {
		return ContextPreviewResponse{}, err
	}
	cfg, err = s.resolveChatModelConfigLocked(cfg, session.ProviderID, session.Model)
	if err != nil {
		return ContextPreviewResponse{}, err
	}
	if session.SystemPromptFrozen || session.ProjectPromptFrozen || strings.TrimSpace(session.SystemPromptSnapshot) != "" || strings.TrimSpace(session.ProjectPromptSnapshot) != "" {
		cfg.SystemPrompt = BuildFinalSystemPrompt(session.SystemPromptSnapshot, session.ProjectPromptSnapshot)
	}
	preparedContext, prepareErr := llm.PrepareChatContext(cfg, cloneMessages(session.Messages))
	prepared := preparedContext.ContextMessages()
	items := make([]ContextPreviewItem, 0, len(prepared))
	totalChars, totalTokens := 0, 0
	for i, msg := range prepared {
		chars := len([]rune(msg.Content))
		tokens := llm.EstimateContextMessageTokens(msg)
		totalChars += chars
		totalTokens += tokens
		source := "最近消息"
		if msg.Role == "system" && i == 0 {
			source = "系统提示词"
		} else if msg.Role == "system" && strings.Contains(msg.Content, "早期会话摘要") {
			source = "早期摘要"
		}
		items = append(items, ContextPreviewItem{Role: msg.Role, Source: source, Chars: chars, EstimatedTokens: tokens, ContentPreview: llm.CompactContextText(msg.Content, 360)})
	}
	checkpoint, checkpointErr := s.contextCheckpointStatusLocked(session.ID, cfg.ProviderID, cfg.Model)
	if checkpointErr != nil {
		return ContextPreviewResponse{}, checkpointErr
	}
	budget := preparedContext.Budget
	budget.ToolOverheadTokens = toolOverheadTokens
	budget.FixedOverheadTokens += toolOverheadTokens
	budget.TotalTokens += toolOverheadTokens
	budget.NextCompression = budget.NextCompression || budget.TotalTokens > budget.AvailableInputTokens
	totalTokens += toolOverheadTokens
	recentMessages := 0
	for _, msg := range prepared {
		if msg.SourceMessageIndex >= 0 {
			recentMessages++
		}
	}
	return ContextPreviewResponse{
		SessionID: session.ID, ProjectID: session.ProjectID, ProviderID: cfg.ProviderID, Model: cfg.Model,
		ContextMode: cfg.ContextMode, RecentMessages: recentMessages, SummarizeOld: preparedContext.Compressed,
		MessageCount: len(session.Messages), ContextCount: len(items), TotalChars: totalChars, EstimatedTokens: totalTokens,
		MaxContextTokens: budget.MaxContextTokens, OutputReserveTokens: budget.OutputReserveTokens, SafetyMarginTokens: budget.SafetyMarginTokens,
		AvailableInputTokens: budget.AvailableInputTokens, FixedOverheadTokens: budget.FixedOverheadTokens, HistoryTokens: budget.HistoryTokens,
		ToolOverheadTokens:       budget.ToolOverheadTokens,
		CompressionTriggerTokens: budget.CompressionTriggerTokens, CompressionTargetTokens: budget.CompressionTargetTokens,
		NextCompression: budget.NextCompression || prepareErr != nil, LimitsEstimated: budget.LimitsEstimated,
		Checkpoint: checkpoint, Items: items,
	}, nil
}
