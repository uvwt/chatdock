package store

import (
	"strings"
	"unicode"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

type ContextPreviewItem struct {
	Role            string `json:"role"`
	Source          string `json:"source"`
	Chars           int    `json:"chars"`
	EstimatedTokens int    `json:"estimated_tokens"`
	ContentPreview  string `json:"content_preview"`
}

type ContextPreviewResponse struct {
	SessionID       string               `json:"session_id"`
	ProjectID       string               `json:"project_id,omitempty"`
	ContextMode     string               `json:"context_mode"`
	RecentMessages  int                  `json:"recent_messages"`
	SummarizeOld    bool                 `json:"summarize_old"`
	MessageCount    int                  `json:"message_count"`
	ContextCount    int                  `json:"context_count"`
	TotalChars      int                  `json:"total_chars"`
	EstimatedTokens int                  `json:"estimated_tokens"`
	Items           []ContextPreviewItem `json:"items"`
}

func (s *Store) ContextPreview(sessionID string) (ContextPreviewResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	recent, summarize := llm.ContextPlan(cfg)
	prepared := llm.BuildChatContextMessages(cfg, cloneMessages(session.Messages))
	items := make([]ContextPreviewItem, 0, len(prepared))
	totalChars, totalTokens := 0, 0
	for i, msg := range prepared {
		chars := len([]rune(msg.Content))
		tokens := estimateTokens(msg.Content)
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
	return ContextPreviewResponse{SessionID: session.ID, ProjectID: session.ProjectID, ContextMode: cfg.ContextMode, RecentMessages: recent, SummarizeOld: summarize, MessageCount: len(session.Messages), ContextCount: len(items), TotalChars: totalChars, EstimatedTokens: totalTokens, Items: items}, nil
}

func estimateTokens(content string) int {
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
