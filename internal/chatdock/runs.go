package chatdock

import (
	"context"
	"database/sql"
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
	mcpCfg, err := a.activeMCPConfig()
	if err != nil || len(mcpCfg.Servers) == 0 {
		if emit != nil {
			return a.client.Stream(ctx, cfg, history, func(delta llm.StreamDelta) error { return emit("delta", delta) })
		}
		return a.client.Complete(ctx, cfg, history)
	}
	tools, err := a.mcpClient.ListTools(ctx, mcpCfg)
	if err != nil || len(tools) == 0 {
		if emit != nil {
			message := "MCP 工具未接入"
			if err != nil {
				message = err.Error()
			}
			if emitErr := emit("tool_setup_error", map[string]any{"message": message}); emitErr != nil {
				return "", emitErr
			}
			return a.client.Stream(ctx, cfg, history, func(delta llm.StreamDelta) error { return emit("delta", delta) })
		}
		return a.client.Complete(ctx, cfg, history)
	}
	if emit != nil {
		if err := emit("tool_setup_ready", map[string]any{"tool_count": len(tools)}); err != nil {
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
	answer, runErr := a.client.CompleteWithMCPToolsEvents(ctx, cfg, history, tools, func(name string, args map[string]any) (any, error) {
		if mcpToolNeedsConfirmation(mcpCfg, name) {
			if err := a.requestMCPConfirmation(ctx, sessionID, name, args, recordingEmit); err != nil {
				return nil, err
			}
			return a.mcpClient.CallToolAfterConfirmation(ctx, mcpCfg, name, args)
		}
		return a.mcpClient.CallTool(ctx, mcpCfg, name, args)
	}, recordingEmit)
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
