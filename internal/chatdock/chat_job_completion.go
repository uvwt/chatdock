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

func (a *App) finishChatJob(ctx context.Context, sessionID string, jobID string, status string, cfg model.ModelConfig, recorder *assistantOutputRecorder, runErr error) {
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
	a.startSessionTitleGeneration(requestIDFromContext(ctx), sessionID, cfg)
}

func (a *App) startSessionTitleGeneration(requestID string, sessionID string, cfg model.ModelConfig) {
	a.jobMu.Lock()
	if a.closing {
		a.jobMu.Unlock()
		return
	}
	a.backgroundWG.Add(1)
	a.jobMu.Unlock()

	go func() {
		defer a.backgroundWG.Done()
		titleCtx := withRequestID(a.lifecycleCtx, requestID)
		if _, err := a.maybeGenerateSessionTitle(titleCtx, sessionID, cfg); err != nil && !isClientCanceled(titleCtx, err) {
			logError("session_title_generation_failed", err, logFields{"request_id": requestID, "session_id": sessionID})
		}
	}()
}
