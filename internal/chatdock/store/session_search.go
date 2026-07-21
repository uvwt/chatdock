package store

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"chatdock/internal/chatdock/llm"
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

func (s *Store) SearchSessions(workspaceID string, query string, limit int) ([]SessionSearchResult, error) {
	items, _, _, err := s.SearchSessionPage(workspaceID, query, "", limit)
	return items, err
}

func (s *Store) SearchSessionPage(workspaceID string, query string, cursor string, limit int) ([]SessionSearchResult, string, bool, error) {
	limit = normalizeSessionPageLimit(limit)
	offset := 0
	if strings.TrimSpace(cursor) != "" {
		parsed, err := strconv.Atoi(cursor)
		if err != nil || parsed < 0 {
			return nil, "", false, fmt.Errorf("invalid search cursor")
		}
		offset = parsed
	}
	results, err := s.searchSessions(workspaceID, query)
	if err != nil {
		return nil, "", false, err
	}
	if offset >= len(results) {
		return []SessionSearchResult{}, "", false, nil
	}
	end := offset + limit
	if end > len(results) {
		end = len(results)
	}
	hasMore := end < len(results)
	nextCursor := ""
	if hasMore {
		nextCursor = strconv.Itoa(end)
	}
	return results[offset:end], nextCursor, hasMore, nil
}

func (s *Store) searchSessions(workspaceID string, query string) ([]SessionSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SessionSearchResult{}, nil
	}
	needle := strings.ToLower(query)
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	sessions, err := loadSessionsFromTablesLocked(s.db, workspaceID)
	if err != nil {
		return nil, err
	}
	scheduledSessionIDs, err := s.scheduledSessionIDsLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	attachmentTextBySession, err := s.attachmentSearchTextBySessionLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	results := make([]SessionSearchResult, 0)
	for _, session := range sessions {
		if _, scheduled := scheduledSessionIDs[session.ID]; scheduled {
			continue
		}
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
		for _, attachment := range attachmentTextBySession[session.ID] {
			if score, f, snip := matchSessionText(attachment.filename, "附件", needle, query); score > bestScore {
				bestScore, field, snippet = score, f, snip
			}
			if score, f, snip := matchSessionText(attachment.textContent, "附件文本", needle, query); score > bestScore {
				bestScore, field, snippet = score, f, snip
			}
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
	return results, nil
}

type attachmentSearchText struct {
	filename    string
	textContent string
}

func (s *Store) attachmentSearchTextBySessionLocked(workspaceID string) (map[string][]attachmentSearchText, error) {
	rows, err := s.db.Query(`SELECT session_id, filename, text_content FROM attachments WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bySession := make(map[string][]attachmentSearchText)
	for rows.Next() {
		var sessionID, filename, textContent string
		if err := rows.Scan(&sessionID, &filename, &textContent); err != nil {
			return nil, err
		}
		bySession[sessionID] = append(bySession[sessionID], attachmentSearchText{filename: filename, textContent: textContent})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bySession, nil
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
	runeIndex := utf8.RuneCountInString(lower[:idx])
	return score, field, makeSearchSnippet(text, runeIndex, utf8.RuneCountInString(needle))
}

func makeSearchSnippet(text string, runeIndex int, queryRunes int) string {
	runes := []rune(text)
	start := runeIndex - 28
	if start < 0 {
		start = 0
	}
	end := runeIndex + queryRunes + 48
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
