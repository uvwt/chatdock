package chatdock

import (
	"context"

	"chatdock/internal/chatdock/model"
)

type scheduledSessionCompletionResult struct {
	Answer         string
	Reasoning      string
	AssistantSaved bool
}

func (a *App) completeScheduledSessionWithRecordedEvents(ctx context.Context, workspaceID string, sessionID string, cfg model.ModelConfig, history []model.Message) (scheduledSessionCompletionResult, error) {
	recorder := newAssistantOutputRecorder(a, workspaceID, sessionID, "")
	fallbackCfg := a.resolveFallbackModelConfig(ctx, sessionID, cfg)
	finalAnswer, _, runErr := a.completeWithRecordedTools(ctx, workspaceID, "", sessionID, cfg, fallbackCfg, history, recorder.emit)
	recorder.useFinalAnswer(finalAnswer)
	recorder.ensureFailureAnswer(runErr)
	if runErr != nil {
		recorder.setError(newMessageError(requestIDFromContext(ctx), runErr.Error()))
	}
	if err := recorder.saveCheckpoint(true); err != nil && runErr == nil {
		runErr = err
	}
	return scheduledSessionCompletionResult{
		Answer:         recorder.answerText(),
		Reasoning:      recorder.reasoningText(),
		AssistantSaved: recorder.assistantWasSaved(),
	}, runErr
}
