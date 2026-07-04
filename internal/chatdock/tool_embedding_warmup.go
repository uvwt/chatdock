package chatdock

import (
	"context"
	"log"
	"strings"
	"time"
)

func (a *App) warmToolEmbeddingIndex(parent context.Context) {
	select {
	case <-time.After(3 * time.Second):
	case <-parent.Done():
		return
	}
	cfg := a.embeddingConfig()
	if strings.TrimSpace(cfg.EmbeddingBaseURL) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	tools := builtinScheduledTaskTools()
	mcpCfg, err := a.activeMCPConfig()
	if err != nil {
		log.Printf("tool embedding warmup using builtin tools only: MCP config unavailable: %v", err)
	} else if len(mcpCfg.Servers) > 0 {
		mcpTools, err := a.mcpClient.ListTools(ctx, mcpCfg)
		if err != nil {
			log.Printf("tool embedding warmup using builtin tools only: list MCP tools failed: %v", err)
		} else {
			tools = append(tools, mcpTools...)
		}
	}
	catalog := newToolCatalog(tools)
	if len(catalog.tools) == 0 {
		return
	}
	started := time.Now()
	records, err := a.ensureToolEmbeddingIndex(ctx, catalog, cfg.EmbeddingModel)
	if err != nil {
		log.Printf("tool embedding warmup failed: %v", err)
		return
	}
	log.Printf("tool embedding warmup complete: tools=%d indexed=%d elapsed=%s", len(catalog.tools), len(records), time.Since(started).Round(time.Millisecond))
}
