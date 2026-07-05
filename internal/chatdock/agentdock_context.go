package chatdock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatdock/internal/chatdock/model"
)

var agentDockRuntimeContextCache = struct {
	sync.Mutex
	text    string
	expires time.Time
}{}

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
	agentDockRuntimeContextCache.Lock()
	if agentDockRuntimeContextCache.text != "" && now.Before(agentDockRuntimeContextCache.expires) {
		text := agentDockRuntimeContextCache.text
		agentDockRuntimeContextCache.Unlock()
		return text
	}
	stale := agentDockRuntimeContextCache.text
	agentDockRuntimeContextCache.Unlock()

	fetchCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	text, err := fetchAgentDockRuntimeContext(fetchCtx, url)
	if err != nil || strings.TrimSpace(text) == "" {
		return stale
	}
	text = compactPreflightText(text, 12000)
	agentDockRuntimeContextCache.Lock()
	agentDockRuntimeContextCache.text = text
	agentDockRuntimeContextCache.expires = now.Add(5 * time.Minute)
	agentDockRuntimeContextCache.Unlock()
	return text
}

func fetchAgentDockRuntimeContext(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
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
