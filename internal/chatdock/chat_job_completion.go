package chatdock

import (
	"context"
	"time"

	"chatdock/internal/chatdock/model"
)

func chatJobCompletionStatus(ctx context.Context, runErr error) (string, error) {
	if isClientCanceled(ctx, runErr) {
		if runErr == nil {
			runErr = ctx.Err()
		}
		return "interrupted", runErr
	}
	if runErr != nil {
		return "failed", runErr
	}
	return "success", nil
}

func (a *App) finishChatJob(ctx context.Context, workspaceID string, sessionID string, jobID string, status string, cfg model.ModelConfig, recorder *assistantOutputRecorder, runErr error) {
	finishedJob, finishErr := a.store.FinishChatJob(jobID, status, recorder.answerText(), recorder.reasoningText(), runErr)
	fields := logFields{
		"request_id":  requestIDFromContext(ctx),
		"job_id":      jobID,
		"session_id":  sessionID,
		"status":      status,
		"provider_id": cfg.ProviderID,
		"model":       cfg.Model,
	}
	if finishErr != nil {
		logError("chat_job_finish_failed", finishErr, fields)
		return
	}
	fields["duration_ms"] = time.Since(finishedJob.StartedAt).Milliseconds()
	if runErr != nil {
		logError("chat_job_failed", runErr, fields)
		return
	}
	logInfo("chat_job_finished", fields)
	go a.generateSessionTitleAfterChat(ctx, workspaceID, sessionID, cfg)
}

func (a *App) generateSessionTitleAfterChat(ctx context.Context, workspaceID string, sessionID string, cfg model.ModelConfig) {
	titleCtx, cancel := context.WithTimeout(withRequestID(context.Background(), requestIDFromContext(ctx)), 20*time.Second)
	defer cancel()
	if _, err := a.maybeGenerateSessionTitle(titleCtx, workspaceID, sessionID, cfg); err != nil {
		logError("session_title_generation_failed", err, logFields{"request_id": requestIDFromContext(ctx), "session_id": sessionID})
	}
}
