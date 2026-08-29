package httpapi

import (
	"fmt"
	"net/http"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

func (a *Server) handleProviderSystemPrompt(w http.ResponseWriter, r *http.Request) {
	cfg, history, err := a.store.SessionProviderContext(r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == model.ErrSessionNotFound {
			status = http.StatusNotFound
		}
		writeError(w, status, fmt.Errorf("provider system prompt failed: %w", err))
		return
	}

	// 与真实模型调用保持同一顺序：先准备附件模型地址并注入 AgentDock
	// 运行时上下文，再按当前工具暴露状态追加工具提示，最后合并 system 消息。
	history = a.prepareVisionAttachmentURLs(history)
	if a.agentDock != nil {
		history = a.agentDock.AppendRuntimeContext(r.Context(), history)
	}
	toolSet, _, err := a.loadConversationTools(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("provider system prompt tools failed: %w", err))
		return
	}

	prompt := llm.BuildProviderSystemPrompt(cfg, history, toolSet.tools(), toolSet.serverInstructions)
	writeJSONResponse(w, http.StatusOK, map[string]string{"system_prompt": prompt})
}
