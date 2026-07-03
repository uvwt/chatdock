package chatdock

import (
	"fmt"
	"net/http"
	"strings"

	"chatdock/internal/chatdock/model"
)

func (a *App) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var input model.CreatePromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.store.CreatePrompt(input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleWorkspaceRoute(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workspaces/"), "/")
	parts := strings.Split(requestPath, "/")
	if len(parts) != 1 && len(parts) != 2 {
		writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
		return
	}

	workspaceID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if _, err := a.store.DeletePrompt(model.SelectPromptRequest{Name: workspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.ListWorkspaces()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
		return
	}
	// 配置中心使用 /api/workspaces 作为产品化资源入口；旧 /api/prompts 继续兼容侧栏提示词空间。
	if parts[1] == "select" && r.Method == http.MethodPost {
		if _, err := a.store.SelectPrompt(model.SelectPromptRequest{Name: workspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.ListWorkspaces()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if parts[1] == "config" && r.Method == http.MethodGet {
		cfg, err := a.store.WorkspaceConfig(workspaceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, cfg)
		return
	}
	if parts[1] == "config" && r.Method == http.MethodPost {
		var input model.ModelConfig
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg, err := a.store.SaveWorkspaceConfig(workspaceID, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, cfg)
		return
	}
	if len(parts) == 2 && parts[1] == "prompt-preview" && r.Method == http.MethodGet {
		preview, err := a.store.PromptPreview(workspaceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, preview)
		return
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("workspace route not found"))
}
