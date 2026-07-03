package chatdock

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"
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
	Workspace       string               `json:"workspace"`
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
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return ContextPreviewResponse{}, ErrSessionNotFound
	}
	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return ContextPreviewResponse{}, err
	}
	cfg.Skills = skills
	recent, summarize := contextPlan(cfg)
	prepared := buildChatContextMessages(cfg, cloneMessages(session.Messages))
	items := make([]ContextPreviewItem, 0, len(prepared))
	totalChars, totalTokens := 0, 0
	for i, msg := range prepared {
		chars := len([]rune(msg.Content))
		tokens := estimateTokens(msg.Content)
		totalChars += chars
		totalTokens += tokens
		source := "最近消息"
		if msg.Role == "system" && i == 0 {
			source = "系统提示词 + 技能"
		} else if msg.Role == "system" && strings.Contains(msg.Content, "早期会话摘要") {
			source = "早期摘要"
		}
		items = append(items, ContextPreviewItem{Role: msg.Role, Source: source, Chars: chars, EstimatedTokens: tokens, ContentPreview: compactContextText(msg.Content, 360)})
	}
	return ContextPreviewResponse{SessionID: session.ID, Workspace: s.activePrompt, ContextMode: cfg.ContextMode, RecentMessages: recent, SummarizeOld: summarize, MessageCount: len(session.Messages), ContextCount: len(items), TotalChars: totalChars, EstimatedTokens: totalTokens, Items: items}, nil
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

func (a *App) handleContextPreview(w http.ResponseWriter, r *http.Request) {
	preview, err := a.store.ContextPreview(r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == ErrSessionNotFound {
			status = http.StatusNotFound
		}
		writeError(w, status, fmt.Errorf("context preview failed: %w", err))
		return
	}
	writeJSONResponse(w, http.StatusOK, preview)
}
