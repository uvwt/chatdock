package chatdock

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
)

type activeToolRun struct {
	RunID     string
	Created   bool
	LastArgs  map[string]any
	StartedAt map[string]time.Time
}

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	result, err := a.store.ListMCPRuns(r.URL.Query().Get("session_id"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.MCPRunDetail(r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == sql.ErrNoRows {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleListAgentTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	result, err := a.store.ListAgentTasks(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) completeWithRecordedTools(ctx context.Context, sessionID string, cfg model.ModelConfig, history []model.Message, emit func(string, any) error) (string, error) {
	// ChatDock 内置工具不依赖 MCP 配置。即使用户没有接入任何 MCP，模型也能管理本工作空间的定时任务。
	tools := builtinScheduledTaskTools()
	mcpCfg, mcpErr := a.activeMCPConfig()
	mcpReady := mcpErr == nil && len(mcpCfg.Servers) > 0
	if mcpReady {
		mcpTools, err := a.mcpClient.ListTools(ctx, mcpCfg)
		if err != nil {
			mcpReady = false
			if emit != nil {
				if emitErr := emit("tool_setup_error", map[string]any{"message": err.Error()}); emitErr != nil {
					return "", emitErr
				}
			}
		} else {
			tools = append(tools, mcpTools...)
		}
	}
	if len(tools) == 0 {
		if emit != nil {
			return a.client.Stream(ctx, cfg, history, func(delta llm.StreamDelta) error { return emit("delta", delta) })
		}
		return a.client.Complete(ctx, cfg, history)
	}
	if emit != nil {
		if err := emit("tool_setup_ready", map[string]any{"tool_count": len(tools), "builtin_tool_count": len(builtinScheduledTaskTools())}); err != nil {
			return "", err
		}
	}
	recorder := &activeToolRun{LastArgs: map[string]any{}, StartedAt: map[string]time.Time{}}
	recordingEmit := func(event string, value any) error {
		if event == "tool_call_start" || event == "tool_call_result" {
			if err := a.recordToolRunEvent(sessionID, recorder, event, value, emit); err != nil {
				return err
			}
		}
		if emit != nil {
			return emit(event, value)
		}
		return nil
	}
	toolEmit := recordingEmit
	if emit == nil {
		// 非流式 /api/chat 不需要把最终回答改成流式请求；工具仍会执行，只是不记录前端运行事件。
		toolEmit = nil
	}
	answer, runErr := a.client.CompleteWithMCPToolsEvents(ctx, cfg, history, tools, func(name string, args map[string]any) (any, error) {
		if isBuiltinScheduledTaskTool(name) {
			return a.callBuiltinScheduledTaskTool(ctx, name, args)
		}
		if !mcpReady {
			return nil, fmt.Errorf("MCP tool is not available: %s", name)
		}
		if mcpToolNeedsConfirmation(mcpCfg, name) {
			if err := a.requestMCPConfirmation(ctx, sessionID, name, args, recordingEmit); err != nil {
				return nil, err
			}
			return a.mcpClient.CallToolAfterConfirmation(ctx, mcpCfg, name, args)
		}
		return a.mcpClient.CallTool(ctx, mcpCfg, name, args)
	}, toolEmit)
	if recorder.Created {
		status := "success"
		if runErr != nil {
			status = "failed"
		}
		run, err := a.store.FinishMCPRun(recorder.RunID, status, "tool run finished", runErr)
		if err == nil && emit != nil {
			_ = emit("run_finish", run)
		}
	}
	return answer, runErr
}

func (a *App) recordToolRunEvent(sessionID string, recorder *activeToolRun, event string, value any, emit func(string, any) error) error {
	data, _ := value.(map[string]any)
	toolName, _ := data["tool"].(string)
	if !recorder.Created {
		run, err := a.store.StartMCPRun(sessionID, "chat tool run")
		if err != nil {
			return err
		}
		recorder.RunID = run.ID
		recorder.Created = true
		if emit != nil {
			_ = emit("run_start", run)
		}
	}
	now := time.Now()
	if event == "tool_call_start" {
		args, _ := data["arguments"].(map[string]any)
		recorder.LastArgs[toolName] = args
		recorder.StartedAt[toolName] = now
		created, err := a.store.AddMCPRunEvent(recorder.RunID, storepkg.RunEventInput{Kind: "tool_call", Status: "running", Tool: toolName, Arguments: args, StartedAt: now})
		if err != nil {
			return err
		}
		if emit != nil {
			_ = emit("run_event", created)
		}
		return nil
	}
	if event != "tool_call_result" {
		return nil
	}
	args := recorder.LastArgs[toolName]
	started := recorder.StartedAt[toolName]
	if started.IsZero() {
		started = now
	}
	status := "success"
	errorText, _ := data["error"].(string)
	if errorText != "" {
		status = "failed"
	}
	finished := now
	created, err := a.store.AddMCPRunEvent(recorder.RunID, storepkg.RunEventInput{Kind: "tool_result", Status: status, Tool: toolName, Arguments: args, Result: data, Error: errorText, StartedAt: started, FinishedAt: &finished, DurationMS: finished.Sub(started).Milliseconds()})
	if err != nil {
		return err
	}
	if emit != nil {
		_ = emit("run_event", created)
	}
	return nil
}
