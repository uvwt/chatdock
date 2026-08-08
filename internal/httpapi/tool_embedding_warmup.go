package httpapi

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"
)

func (a *Server) warmToolEmbeddingIndex(parent context.Context) {
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
	tools := builtinChatDockTools()
	mcpCfg, err := a.activeMCPConfig()
	if err != nil {
		log.Printf("tool embedding warmup using builtin tools only: MCP config unavailable: %v", err)
	} else if len(mcpCfg.Servers) > 0 {
		serverNames := make([]string, 0, len(mcpCfg.Servers))
		for serverName, server := range mcpCfg.Servers {
			if mcpServerNeedsInitialLoad(server) {
				serverNames = append(serverNames, serverName)
			}
		}
		sort.Strings(serverNames)
		for _, serverName := range serverNames {
			mcpTools, listErr := a.mcpClient.ListServerTools(ctx, mcpCfg, serverName)
			if listErr != nil {
				log.Printf("tool embedding warmup skipped MCP resource %s: %v", serverName, listErr)
				continue
			}
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
