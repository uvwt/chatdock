package httpapi

import (
	"context"
	"time"

	"chatdock/internal/llm"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

type activeToolRun struct {
	RunID     string
	Created   bool
	LastArgs  map[string]any
	StartedAt map[string]time.Time
}

func (a *Server) completeWithRecordedTools(ctx context.Context, jobID string, sessionID string, cfg model.ModelConfig, fallbackCfg *model.ModelConfig, history []model.Message, emit func(string, any) error) (string, model.ModelConfig, error) {
	toolMessageIndexes := llm.HistoricalToolMessageIndexes(history)
	if err := a.store.HydrateMessageEventDetails(sessionID, history, toolMessageIndexes); err != nil {
		return "", cfg, err
	}
	history = a.prepareVisionAttachmentURLs(history)
	if a.agentDock != nil {
		history = a.agentDock.AppendRuntimeContext(ctx, history)
	}

	toolSet, mcpConfig, err := a.loadConversationTools(ctx, emit)
	if err != nil {
		return "", cfg, err
	}
	visibleTools := toolSet.tools()

	if emit != nil {
		if err := emit("tool_setup_ready", map[string]any{
			"mode":                     "resource_dynamic",
			"tool_count":               len(toolSet.loaded.tools),
			"exposed_tool_count":       len(visibleTools),
			"builtin_tool_count":       len(builtinChatDockTools()),
			"on_demand_tool_count":     len(toolSet.onDemand.tools),
			"resource_count":           len(toolSet.resources.byID),
			"loaded_resource_count":    toolSet.resources.loadedCount(),
			"on_demand_resource_count": toolSet.resources.onDemandCount(),
			"resource_error_count":     toolSet.resources.errorCount(),
		}); err != nil {
			return "", cfg, err
		}
	}
	recorder := &activeToolRun{LastArgs: map[string]any{}, StartedAt: map[string]time.Time{}}
	recordingEmit := a.toolRunEmitter(sessionID, recorder, emit)
	runRealTool := func(name string, args map[string]any) (any, error) {
		return a.callConversationTool(ctx, sessionID, mcpConfig, name, args, recordingEmit)
	}

	toolEmit := recordingEmit
	if emit == nil {
		toolEmit = nil
	}

	answer, usedCfg, runErr := completeModelWithFallback(ctx, cfg, fallbackCfg, toolEmit, func(attemptCfg model.ModelConfig, attemptEmit func(string, any) error, markStarted func()) (string, error) {
		return a.client.CompleteWithMCPToolsEvents(ctx, attemptCfg, history, visibleTools, func(name string, args map[string]any) (any, error) {
			return a.callVisibleConversationTool(ctx, toolSet, runRealTool, name, args)
		}, attemptEmit, llm.MCPToolLoopOptions{
			RefreshTools:       toolSet.tools,
			OnToolCall:         markStarted,
			ServerInstructions: toolSet.serverInstructions,
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

func (a *Server) toolRunEmitter(sessionID string, recorder *activeToolRun, emit func(string, any) error) func(string, any) error {
	return func(event string, value any) error {
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
}

func (a *Server) finishRecordedToolRun(recorder *activeToolRun, runErr error, emit func(string, any) error) error {
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

func compactMCPAppForRunAudit(data map[string]any) map[string]any {
	if len(data) == 0 {
		return data
	}
	out := make(map[string]any, len(data))
	for key, value := range data {
		if key != "mcp_app" {
			out[key] = value
			continue
		}
		app, ok := value.(map[string]any)
		if !ok {
			continue
		}
		descriptor := map[string]any{}
		for _, field := range []string{"server", "resource_uri", "mime_type"} {
			if item, exists := app[field]; exists {
				descriptor[field] = item
			}
		}
		if len(descriptor) > 0 {
			out[key] = descriptor
		}
	}
	return out
}

func (a *Server) recordToolRunEvent(sessionID string, recorder *activeToolRun, event string, value any, emit func(string, any) error) error {
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
	created, err := a.store.AddMCPRunEvent(recorder.RunID, storepkg.RunEventInput{Kind: "tool_result", Status: status, Tool: toolName, Arguments: args, Result: compactMCPAppForRunAudit(data), Error: errorText, StartedAt: started, FinishedAt: &finished, DurationMS: finished.Sub(started).Milliseconds()})
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
