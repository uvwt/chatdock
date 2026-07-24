package chatdock

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"chatdock/internal/chatdock/agentdock"
	"chatdock/internal/chatdock/model"
)

func (a *App) handleListAgentTasks(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		switch status {
		case "active", "blocked", "completed":
			query.Set("status", status)
		default:
			writeJSONResponse(w, http.StatusBadRequest, map[string]any{"code": "INVALID_STATUS", "error": "任务状态必须是 active、blocked 或 completed"})
			return
		}
	}
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > 100 {
			writeJSONResponse(w, http.StatusBadRequest, map[string]any{"code": "INVALID_LIMIT", "error": "任务数量必须是 1 到 100 之间的整数"})
			return
		}
		query.Set("limit", strconv.Itoa(limit))
	}
	a.proxyAgentDockTasks(w, r, "/internal/runtime/tasks", query)
}

func (a *App) handleGetAgentTask(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("id"))
	if taskID == "" {
		writeJSONResponse(w, http.StatusBadRequest, map[string]any{"code": "TASK_ID_REQUIRED", "error": "任务 ID 不能为空"})
		return
	}
	a.proxyAgentDockTasks(w, r, "/internal/runtime/tasks/"+url.PathEscape(taskID), nil)
}

func (a *App) handleDeleteAgentTask(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("id"))
	if taskID == "" {
		writeJSONResponse(w, http.StatusBadRequest, map[string]any{"code": "TASK_ID_REQUIRED", "error": "任务 ID 不能为空"})
		return
	}
	a.proxyAgentDockTaskRequest(w, r, http.MethodDelete, "/internal/runtime/tasks/"+url.PathEscape(taskID), nil, false)
}

func (a *App) handleGetSessionAgentTask(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("id"))
	if sessionID == "" {
		writeJSONResponse(w, http.StatusBadRequest, map[string]any{"code": "SESSION_ID_REQUIRED", "error": "会话 ID 不能为空"})
		return
	}

	session, ok, err := a.store.GetSession(sessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, model.ErrSessionNotFound)
		return
	}

	taskID, err := a.latestSessionAgentTaskID(session)
	if err != nil {
		writeJSONResponse(w, http.StatusInternalServerError, map[string]any{"code": "SESSION_TASK_LOOKUP_FAILED", "error": err.Error()})
		return
	}
	if taskID == "" {
		writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "task": nil})
		return
	}
	a.proxyAgentDockTaskRequest(w, r, http.MethodGet, "/internal/runtime/tasks/"+url.PathEscape(taskID), nil, true)
}

func (a *App) latestSessionAgentTaskID(session *model.Session) (string, error) {
	for messageIndex := len(session.Messages) - 1; messageIndex >= 0; messageIndex-- {
		events := session.Messages[messageIndex].Events
		for eventIndex := len(events) - 1; eventIndex >= 0; eventIndex-- {
			event := events[eventIndex]
			if event.Kind != "tool" || !agentdock.LooksLikeTaskManageEvent(event) {
				continue
			}
			fullEvent, err := a.store.SessionMessageEventByID(session.ID, event.ID)
			if err != nil {
				return "", fmt.Errorf("读取会话任务事件失败: %w", err)
			}
			if taskID := agentdock.SessionTaskIDFromEvent(fullEvent); taskID != "" {
				return taskID, nil
			}
		}
	}
	return "", nil
}

func (a *App) proxyAgentDockTasks(w http.ResponseWriter, r *http.Request, runtimePath string, query url.Values) {
	a.proxyAgentDockTaskRequest(w, r, http.MethodGet, runtimePath, query, false)
}

func (a *App) proxyAgentDockTaskRequest(w http.ResponseWriter, r *http.Request, method, runtimePath string, query url.Values, notFoundAsEmptyTask bool) {
	if a.agentDock == nil || !a.agentDock.Configured() {
		writeJSONResponse(w, http.StatusServiceUnavailable, map[string]any{"code": "AGENTDOCK_NOT_CONFIGURED", "error": "AgentDock 任务接口尚未配置"})
		return
	}

	payload, upstreamStatus, err := a.agentDock.RequestTaskJSON(r.Context(), method, runtimePath, query)
	if err != nil {
		writeJSONResponse(w, http.StatusBadGateway, map[string]any{"code": "AGENTDOCK_TASKS_UNAVAILABLE", "error": "访问 AgentDock 任务失败：" + err.Error()})
		return
	}
	if upstreamStatus == http.StatusNotFound && notFoundAsEmptyTask {
		// 会话历史仍可能引用已被用户删除的任务。此时当前会话应回到“无任务”，
		// 而不是持续显示一个无法恢复的 404 错误卡片。
		writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "task": nil})
		return
	}
	if upstreamStatus < 200 || upstreamStatus >= 300 {
		status := http.StatusBadGateway
		if upstreamStatus == http.StatusBadRequest {
			status = http.StatusBadRequest
		} else if upstreamStatus == http.StatusNotFound {
			status = http.StatusNotFound
		}
		message := "AgentDock 任务接口返回异常"
		if value, ok := payload["error"].(string); ok && strings.TrimSpace(value) != "" {
			message = value
		}
		writeJSONResponse(w, status, map[string]any{"code": "AGENTDOCK_TASKS_UPSTREAM_ERROR", "error": message})
		return
	}
	writeJSONResponse(w, http.StatusOK, payload)
}
