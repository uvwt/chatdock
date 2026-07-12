package chatdock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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

func (a *App) proxyAgentDockTasks(w http.ResponseWriter, r *http.Request, runtimePath string, query url.Values) {
	contextURL := strings.TrimSpace(a.cfg.AgentDockContextURL)
	if contextURL == "" {
		writeJSONResponse(w, http.StatusServiceUnavailable, map[string]any{"code": "AGENTDOCK_NOT_CONFIGURED", "error": "AgentDock 任务接口尚未配置"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	payload, upstreamStatus, err := fetchAgentDockRuntimeJSON(ctx, contextURL, a.cfg.AgentDockContextToken, runtimePath, query)
	if err != nil {
		writeJSONResponse(w, http.StatusBadGateway, map[string]any{"code": "AGENTDOCK_TASKS_UNAVAILABLE", "error": "读取 AgentDock 任务失败：" + err.Error()})
		return
	}
	if upstreamStatus < 200 || upstreamStatus >= 300 {
		status := http.StatusBadGateway
		if upstreamStatus == http.StatusNotFound {
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

func fetchAgentDockRuntimeJSON(ctx context.Context, contextURL, token, runtimePath string, query url.Values) (map[string]any, int, error) {
	target, err := agentDockRuntimeURL(contextURL, runtimePath, query)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, agentDockTaskResponseLimit)).Decode(&payload); err != nil {
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

	// Context 与只读 Runtime API 由同一个 AgentDock 服务提供；复用现有配置，
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
