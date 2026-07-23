package chatdock

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

const agentDockTaskResponseLimit = 2 * 1024 * 1024

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
			if event.Kind != "tool" || !looksLikeTaskManageEvent(event) {
				continue
			}
			fullEvent, err := a.store.SessionMessageEventByID(session.ID, event.ID)
			if err != nil {
				return "", fmt.Errorf("读取会话任务事件失败: %w", err)
			}
			if taskID := sessionTaskIDFromEvent(fullEvent); taskID != "" {
				return taskID, nil
			}
		}
	}
	return "", nil
}

func looksLikeTaskManageEvent(event model.MessageEvent) bool {
	text := strings.ToLower(strings.TrimSpace(event.Meta + " " + event.Text))
	return strings.Contains(text, "task_manage")
}

func sessionTaskIDFromEvent(event model.MessageEvent) string {
	details := event.Details
	data := mapValue(details["data"])
	tool := firstMessagePartNonEmpty(stringValue(details["tool"]), stringValue(data["tool"]))
	outerArgs := mapValue(details["arguments"])
	if len(outerArgs) == 0 {
		outerArgs = mapValue(data["arguments"])
	}
	outerResult := mapValue(details["result"])
	if len(outerResult) == 0 {
		outerResult = mapValue(data["result"])
	}

	actualTool := tool
	actualArgs := outerArgs
	actualResult := outerResult
	if tool == "chatdock_tool_execute" {
		actualTool = firstMessagePartNonEmpty(stringValue(outerArgs["name"]), stringValue(outerResult["tool"]))
		actualArgs = mapValue(outerArgs["arguments"])
		actualResult = mapValue(outerResult["result"])
	}
	if !isTaskManageTool(actualTool) || !isSessionTaskAction(stringValue(actualArgs["action"])) {
		return ""
	}
	return firstMessagePartNonEmpty(
		stringValue(actualResult["task_id"]),
		stringValue(mapValue(actualResult["task_summary"])["id"]),
		stringValue(actualArgs["task_id"]),
	)
}

func isTaskManageTool(name string) bool {
	name = strings.TrimSpace(name)
	return name == "task_manage" || strings.HasSuffix(name, ".task_manage") || strings.HasSuffix(name, "__task_manage")
}

func isSessionTaskAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "create", "checkpoint", "block", "resume", "final_review", "complete":
		return true
	default:
		return false
	}
}

func (a *App) proxyAgentDockTasks(w http.ResponseWriter, r *http.Request, runtimePath string, query url.Values) {
	a.proxyAgentDockTaskRequest(w, r, http.MethodGet, runtimePath, query, false)
}

func (a *App) proxyAgentDockTaskRequest(w http.ResponseWriter, r *http.Request, method, runtimePath string, query url.Values, notFoundAsEmptyTask bool) {
	contextURL := strings.TrimSpace(a.cfg.AgentDockContextURL)
	if contextURL == "" {
		writeJSONResponse(w, http.StatusServiceUnavailable, map[string]any{"code": "AGENTDOCK_NOT_CONFIGURED", "error": "AgentDock 任务接口尚未配置"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	payload, upstreamStatus, err := requestAgentDockRuntimeJSON(ctx, contextURL, a.cfg.AgentDockContextToken, method, runtimePath, query)
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

func requestAgentDockRuntimeJSON(ctx context.Context, contextURL, token, method, runtimePath string, query url.Values) (map[string]any, int, error) {
	target, err := agentDockRuntimeURL(contextURL, runtimePath, query)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := decodeBoundedJSON(resp.Body, agentDockTaskResponseLimit, &payload); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("解析 AgentDock 任务响应失败: %w", err)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, resp.StatusCode, nil
}

func agentDockRuntimeURL(contextURL, runtimePath string, query url.Values) (string, error) {
	target, err := url.Parse(strings.TrimSpace(contextURL))
	if err != nil {
		return "", fmt.Errorf("AgentDock Context URL 无效: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
		return "", fmt.Errorf("AgentDock Context URL 必须是完整的 HTTP 地址")
	}

	// Context 与任务 Runtime API 由同一个 AgentDock 服务提供；复用现有配置，
	// 避免为了任务面板再维护一套地址和 Token。
	prefix := strings.TrimSuffix(target.Path, "/")
	if strings.HasSuffix(prefix, "/context") {
		prefix = strings.TrimSuffix(prefix, "/context")
	}
	target.Path = strings.TrimRight(prefix, "/") + runtimePath
	target.RawPath = ""
	target.RawQuery = query.Encode()
	target.Fragment = ""
	return target.String(), nil
}
