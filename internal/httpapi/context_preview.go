package httpapi

import (
	"fmt"
	"net/http"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

func (a *Server) handleContextPreview(w http.ResponseWriter, r *http.Request) {
	toolOverhead := 0
	if toolSet, _, toolErr := a.loadConversationTools(r.Context(), nil); toolErr == nil {
		visibleTools := toolSet.tools()
		toolOverhead = llm.EstimateToolsTokens(llm.MCPToolsToOpenAITools(visibleTools)) + llm.EstimateMCPContextTokens(visibleTools, toolSet.serverInstructions)
	}
	preview, err := a.store.ContextPreviewWithToolOverhead(r.PathValue("id"), toolOverhead)
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrSessionNotFound {
			status = http.StatusNotFound
		}
		writeError(w, status, fmt.Errorf("context preview failed: %w", err))
		return
	}
	writeJSONResponse(w, http.StatusOK, preview)
}
