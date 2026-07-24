package chatdock

import (
	"chatdock/internal/chatdock/chatoutput"
	"context"

	"chatdock/internal/chatdock/model"
)

type scheduledSessionCompletionResult struct {
	Answer         string
	Reasoning      string
	AssistantSaved bool
}

func (a *App) completeScheduledSessionWithRecordedEvents(ctx context.Context, sessionID string, cfg model.ModelConfig, history []model.Message) (scheduledSessionCompletionResult, error) {
	recorder := chatoutput.NewRecorder(a.store, sessionID, "")
	fallbackCfg := a.resolveFallbackModelConfig(ctx, sessionID, cfg)
	finalAnswer, _, runErr := a.completeWithRecordedTools(ctx, "", sessionID, cfg, fallbackCfg, history, recorder.Emit)
	recorder.UseFinalAnswer(finalAnswer)
	recorder.EnsureFailureAnswer(runErr)
	if runErr != nil {
		recorder.SetError(newMessageError(requestIDFromContext(ctx), runErr.Error()))
	}
	if err := recorder.SaveCheckpoint(true); err != nil && runErr == nil {
		runErr = err
	}
	return scheduledSessionCompletionResult{
		Answer:         recorder.AnswerText(),
		Reasoning:      recorder.ReasoningText(),
		AssistantSaved: recorder.AssistantWasSaved(),
	}, runErr
}
