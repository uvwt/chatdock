package chatdock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (a *App) appendAgentDockRuntimeContext(ctx context.Context, history []model.Message) []model.Message {
	text := strings.TrimSpace(a.agentDockRuntimeContext(ctx))
	if text == "" {
		return history
	}
	out := append([]model.Message(nil), history...)
	out = append(out, model.Message{Role: "system", Content: text, CreatedAt: time.Now()})
	return out
}

func (a *App) agentDockRuntimeContext(ctx context.Context) string {
	url := strings.TrimSpace(a.cfg.AgentDockContextURL)
	if url == "" {
		return ""
	}
	now := time.Now()
	a.agentDockContextMu.Lock()
	if a.agentDockContext != "" && now.Before(a.agentDockContextUntil) {
		text := a.agentDockContext
		a.agentDockContextMu.Unlock()
		return text
	}
	stale := a.agentDockContext
	a.agentDockContextMu.Unlock()

	fetchCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	text, err := fetchAgentDockRuntimeContext(fetchCtx, url, a.cfg.AgentDockContextToken)
	if err != nil || strings.TrimSpace(text) == "" {
		return stale
	}
	text = compactRuntimeContextText(text, 12000)
	a.agentDockContextMu.Lock()
	a.agentDockContext = text
	a.agentDockContextUntil = now.Add(5 * time.Minute)
	a.agentDockContextMu.Unlock()
	return text
}

func fetchAgentDockRuntimeContext(ctx context.Context, url string, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 2500 * time.Millisecond}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", nil
	}
	var payload struct {
		Context string `json:"context"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Context) != "" {
		return payload.Context, nil
	}
	return payload.Summary, nil
}

func compactRuntimeContextText(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}
