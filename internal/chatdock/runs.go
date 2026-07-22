package chatdock

import (
	"context"
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

func (a *App) completeWithRecordedTools(ctx context.Context, workspaceID string, jobID string, sessionID string, cfg model.ModelConfig, fallbackCfg *model.ModelConfig, history []model.Message, emit func(string, any) error) (string, model.ModelConfig, error) {
	history = a.prepareVisionAttachmentURLs(history)
	history = a.appendAgentDockRuntimeContext(ctx, history)

	allTools, mcpConfig, mcpReady, err := a.loadConversationTools(ctx, workspaceID, emit)
	if err != nil {
		return "", cfg, err
	}
	if len(allTools) == 0 {
		return completeModelWithFallback(ctx, cfg, fallbackCfg, emit, func(attemptCfg model.ModelConfig, attemptEmit func(string, any) error, _ func()) (string, error) {
			if attemptEmit != nil {
				return a.client.Stream(ctx, attemptCfg, history, func(delta llm.StreamDelta) error { return attemptEmit("delta", delta) })
			}
			return a.client.Complete(ctx, attemptCfg, history)
		})
	}

	toolSet := newConversationToolSet(allTools, mcpConfig)
	visibleTools := toolSet.tools()
	if emit != nil {
		if err := emit("tool_setup_ready", map[string]any{"mode": "dynamic", "tool_count": len(allTools), "exposed_tool_count": len(visibleTools), "builtin_tool_count": len(builtinChatDockTools()), "on_demand_tool_count": len(toolSet.onDemand.tools)}); err != nil {
			return "", cfg, err
		}
	}
	recorder := &activeToolRun{LastArgs: map[string]any{}, StartedAt: map[string]time.Time{}}
	recordingEmit := a.toolRunEmitter(workspaceID, sessionID, recorder, emit)
	runRealTool := func(name string, args map[string]any) (any, error) {
		return a.callConversationTool(ctx, workspaceID, sessionID, mcpConfig, mcpReady, name, args, recordingEmit)
	}

	toolEmit := recordingEmit
	if emit == nil {
		toolEmit = nil
	}

	answer, usedCfg, runErr := completeModelWithFallback(ctx, cfg, fallbackCfg, toolEmit, func(attemptCfg model.ModelConfig, attemptEmit func(string, any) error, markStarted func()) (string, error) {
		return a.client.CompleteWithMCPToolsEvents(ctx, attemptCfg, history, visibleTools, func(name string, args map[string]any) (any, error) {
			return a.callVisibleConversationTool(ctx, workspaceID, toolSet, runRealTool, name, args)
		}, attemptEmit, llm.MCPToolLoopOptions{
			RefreshTools: toolSet.tools,
			OnToolCall:   markStarted,
			AfterToolRound: func() ([]map[string]any, error) {
				return a.consumeChatJobGuidance(jobID, emit)
			},
		})
	})
	if finishErr := a.finishRecordedToolRun(recorder, runErr, emit); finishErr != nil && runErr == nil {
		runErr = finishErr
	}
	return answer, usedCfg, runErr
}

func (a *App) toolRunEmitter(workspaceID string, sessionID string, recorder *activeToolRun, emit func(string, any) error) func(string, any) error {
	return func(event string, value any) error {
		if event == "tool_call_start" || event == "tool_call_result" {
			if err := a.recordToolRunEvent(workspaceID, sessionID, recorder, event, value, emit); err != nil {
				return err
			}
		}
		if emit != nil {
			return emit(event, value)
		}
		return nil
	}
}

func (a *App) finishRecordedToolRun(recorder *activeToolRun, runErr error, emit func(string, any) error) error {
	if !recorder.Created {
		return nil
	}
	status := "success"
	if runErr != nil {
		status = "failed"
	}
	run, err := a.store.FinishMCPRun(recorder.RunID, status, "tool run finished", runErr)
	if err != nil {
		return err
	}
	if emit != nil {
		return emit("run_finish", run)
	}
	return nil
}

func (a *App) recordToolRunEvent(workspaceID string, sessionID string, recorder *activeToolRun, event string, value any, emit func(string, any) error) error {
	data, _ := value.(map[string]any)
	toolName, _ := data["tool"].(string)
	if !recorder.Created {
		run, err := a.store.StartMCPRun(workspaceID, sessionID, "chat tool run")
		if err != nil {
			return err
		}
		recorder.RunID = run.ID
		recorder.Created = true
		if emit != nil {
			if err := emit("run_start", run); err != nil {
				return err
			}
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
			if err := emit("run_event", created); err != nil {
				return err
			}
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
		if err := emit("run_event", created); err != nil {
			return err
		}
	}
	return nil
}
