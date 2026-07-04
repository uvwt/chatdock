package chatdock

import (
	"context"
	"strings"
	"time"
)

func (a *App) keepEmbeddingModelWarm(parent context.Context) {
	select {
	case <-time.After(8 * time.Second):
	case <-parent.Done():
		return
	}
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()
	for {
		a.pingEmbeddingModel(parent)
		select {
		case <-ticker.C:
		case <-parent.Done():
			return
		}
	}
}

func (a *App) pingEmbeddingModel(parent context.Context) {
	cfg := a.embeddingConfig()
	if strings.TrimSpace(cfg.EmbeddingBaseURL) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	_, _ = a.client.Embed(ctx, cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, []string{"chatdock embedding keep warm"})
}
