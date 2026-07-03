package chatdock

import (
	"chatdock/internal/chatdock/llm"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type SessionSearchResult struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Pinned       bool      `json:"pinned"`
	Preview      string    `json:"preview,omitempty"`
	LastRole     string    `json:"last_role,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Count        int       `json:"count"`
	MatchField   string    `json:"match_field,omitempty"`
	MatchSnippet string    `json:"match_snippet,omitempty"`
	Score        int       `json:"score"`
}

func (s *Store) SearchSessions(query string, limit int) ([]SessionSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	needle := strings.ToLower(query)
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]SessionSearchResult, 0)
	for _, session := range s.sessions {
		lastRole, preview := sessionPreview(session)
		bestScore, field, snippet := matchSessionText(session.Title, "标题", needle, query)
		if score, f, snip := matchSessionText(preview, "摘要", needle, query); score > bestScore {
			bestScore, field, snippet = score, f, snip
		}
		for _, msg := range session.Messages {
			label := llm.ContextRoleLabel(msg.Role)
			if score, f, snip := matchSessionText(msg.Content, label, needle, query); score > bestScore {
				bestScore, field, snippet = score, f, snip
			}
			for _, att := range msg.Attachments {
				if score, f, snip := matchSessionText(att.Name, "附件", needle, query); score > bestScore {
					bestScore, field, snippet = score, f, snip
				}
			}
		}
		if score, f, snip := s.matchAttachmentTextLocked(session.ID, needle, query); score > bestScore {
			bestScore, field, snippet = score, f, snip
		}
		if bestScore <= 0 {
			continue
		}
		results = append(results, SessionSearchResult{ID: session.ID, Title: session.Title, Pinned: session.Pinned, Preview: preview, LastRole: lastRole, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt, Count: len(session.Messages), MatchField: field, MatchSnippet: snippet, Score: bestScore})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Pinned != results[j].Pinned {
			return results[i].Pinned
		}
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *Store) matchAttachmentTextLocked(sessionID string, needle string, original string) (int, string, string) {
	rows, err := s.db.Query(`SELECT filename, text_content FROM attachments WHERE prompt = ? AND session_id = ?`, s.activePrompt, sessionID)
	if err != nil {
		return 0, "", ""
	}
	defer rows.Close()
	bestScore, bestField, bestSnippet := 0, "", ""
	for rows.Next() {
		var filename, content string
		if err := rows.Scan(&filename, &content); err != nil {
			continue
		}
		if score, field, snippet := matchSessionText(filename, "附件", needle, original); score > bestScore {
			bestScore, bestField, bestSnippet = score, field, snippet
		}
		if score, field, snippet := matchSessionText(content, "附件文本", needle, original); score > bestScore {
			bestScore, bestField, bestSnippet = score, field, snippet
		}
	}
	return bestScore, bestField, bestSnippet
}

func matchSessionText(value string, field string, needle string, original string) (int, string, string) {
	text := strings.TrimSpace(value)
	if text == "" || needle == "" {
		return 0, "", ""
	}
	lower := strings.ToLower(text)
	idx := strings.Index(lower, needle)
	if idx < 0 {
		return 0, "", ""
	}
	score := 10
	if idx == 0 {
		score += 5
	}
	if strings.EqualFold(strings.TrimSpace(text), strings.TrimSpace(original)) {
		score += 20
	}
	return score, field, makeSearchSnippet(text, idx, len([]rune(original)))
}

func makeSearchSnippet(text string, byteIndex int, queryRunes int) string {
	runes := []rune(text)
	prefixRunes := len([]rune(text[:byteIndex]))
	start := prefixRunes - 28
	if start < 0 {
		start = 0
	}
	end := prefixRunes + queryRunes + 48
	if end > len(runes) {
		end = len(runes)
	}
	snippet := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}

func (a *App) handleSearchSessions(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.SearchSessions(r.URL.Query().Get("q"), 80)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("search sessions failed: %w", err))
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"sessions": items})
}
