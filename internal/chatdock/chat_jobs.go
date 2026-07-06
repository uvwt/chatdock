package chatdock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
)

func (a *App) startChatJob(ctx context.Context, input model.ChatRequest) (storepkg.ChatJob, *model.Session, error) {
	var session *model.Session
	var cfg model.ModelConfig
	var history []model.Message
	var err error
	if input.Regenerate {
		session, cfg, history, err = a.store.PrepareSessionRegeneration(input.SessionID)
	} else {
		input.Message = strings.TrimSpace(input.Message)
		if input.Message == "" && len(input.AttachmentIDs) == 0 {
			return storepkg.ChatJob{}, nil, fmt.Errorf("message is empty")
		}
		session, cfg, history, err = a.store.AppendUserMessageWithAttachments(input.SessionID, input.Message, input.AttachmentIDs)
	}
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	cfg, err = a.store.ResolveChatModelConfig(cfg, input.ProviderID, input.Model)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	if _, err := a.store.UpdateSessionModel(input.SessionID, cfg.ProviderID, cfg.Model); err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	requestID := requestIDFromContext(ctx)
	if requestID == "" {
		requestID = newRequestID()
	}
	job, err := a.store.CreateChatJob(input.SessionID, requestID)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	jobCtx, cancel := context.WithCancel(withRequestID(context.Background(), requestID))
	a.registerChatJobCancel(job.ID, cancel)
	logInfo("chat_job_started", logFields{"request_id": requestID, "job_id": job.ID, "session_id": input.SessionID, "provider_id": cfg.ProviderID, "model": cfg.Model})
	go a.runChatJob(jobCtx, job.ID, input.SessionID, cfg, history)
	return job, session, nil
}

func (a *App) registerChatJobCancel(jobID string, cancel context.CancelFunc) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	a.jobCancel[strings.TrimSpace(jobID)] = cancel
}

func (a *App) unregisterChatJobCancel(jobID string) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	delete(a.jobCancel, strings.TrimSpace(jobID))
}

func (a *App) cancelChatJob(jobID string) (storepkg.ChatJob, error) {
	jobID = strings.TrimSpace(jobID)
	a.jobMu.Lock()
	cancel := a.jobCancel[jobID]
	a.jobMu.Unlock()
	if cancel != nil {
		cancel()
	}
	job, err := a.store.InterruptChatJob(jobID, "用户已停止生成。")
	if err == nil {
		_, _ = a.store.AddChatJobEvent(jobID, "job_cancelled", map[string]any{"message": "用户已停止生成。"})
	}
	return job, err
}

func (a *App) runChatJob(ctx context.Context, jobID string, sessionID string, cfg model.ModelConfig, history []model.Message) {
	defer a.unregisterChatJobCancel(jobID)
	defer a.clearChatJobGuidance(jobID)
	var answer strings.Builder
	var reasoning strings.Builder
	var parts messagePartsRecorder
	var checkpointMessageID string
	var lastCheckpoint time.Time
	lastCheckpointChars := 0
	var pendingDelta llm.StreamDelta
	var pendingDeltaChars int
	lastDeltaFlush := time.Now()
	flushDeltaEvent := func(force bool) error {
		if pendingDelta.Content == "" && pendingDelta.ReasoningContent == "" {
			return nil
		}
		if !force && pendingDeltaChars < 512 && time.Since(lastDeltaFlush) < 250*time.Millisecond {
			return nil
		}
		delta := pendingDelta
		pendingDelta = llm.StreamDelta{}
		pendingDeltaChars = 0
		lastDeltaFlush = time.Now()
		_, err := a.store.AddChatJobEvent(jobID, "delta", delta)
		return err
	}
	saveCheckpoint := func(force bool) error {
		currentAnswer := answer.String()
		currentReasoning := reasoning.String()
		if !force && len(currentAnswer)-lastCheckpointChars < 512 && time.Since(lastCheckpoint) < time.Second {
			return nil
		}
		if strings.TrimSpace(currentAnswer) == "" && strings.TrimSpace(currentReasoning) == "" && len(parts.parts) == 0 && len(parts.events) == 0 {
			return nil
		}
		_, messageID, err := a.store.UpsertAssistantMessageCheckpoint(sessionID, checkpointMessageID, currentAnswer, currentReasoning, parts.parts, parts.events)
		if err != nil {
			return err
		}
		checkpointMessageID = messageID
		lastCheckpoint = time.Now()
		lastCheckpointChars = len(currentAnswer)
		return nil
	}
	emit := func(event string, value any) error {
		parts.record(event, value)
		if event == "delta" {
			if delta, ok := value.(llm.StreamDelta); ok {
				if delta.Content != "" {
					answer.WriteString(delta.Content)
					pendingDelta.Content += delta.Content
					pendingDeltaChars += len(delta.Content)
				}
				if delta.ReasoningContent != "" {
					reasoning.WriteString(delta.ReasoningContent)
					pendingDelta.ReasoningContent += delta.ReasoningContent
					pendingDeltaChars += len(delta.ReasoningContent)
				}
				if err := saveCheckpoint(false); err != nil {
					return err
				}
				return flushDeltaEvent(false)
			}
		}
		if err := flushDeltaEvent(true); err != nil {
			return err
		}
		_, err := a.store.AddChatJobEvent(jobID, event, value)
		return err
	}

	finalAnswer, runErr := a.completeWithRecordedTools(ctx, jobID, sessionID, cfg, history, emit)
	if err := flushDeltaEvent(true); err != nil && runErr == nil {
		runErr = err
	}
	if strings.TrimSpace(finalAnswer) != "" && strings.TrimSpace(finalAnswer) != strings.TrimSpace(answer.String()) {
		answer.Reset()
		answer.WriteString(finalAnswer)
	}
	status := "success"
	if isClientCanceled(ctx, runErr) {
		status = "interrupted"
		if runErr == nil {
			runErr = ctx.Err()
		}
	} else if runErr != nil {
		status = "failed"
	}
	if err := saveCheckpoint(true); err != nil {
		status = "failed"
		runErr = err
	}
	finishedJob, finishErr := a.store.FinishChatJob(jobID, status, answer.String(), reasoning.String(), runErr)
	fields := logFields{"request_id": requestIDFromContext(ctx), "job_id": jobID, "session_id": sessionID, "status": status, "provider_id": cfg.ProviderID, "model": cfg.Model}
	if finishErr != nil {
		logError("chat_job_finish_failed", finishErr, fields)
		return
	}
	fields["duration_ms"] = time.Since(finishedJob.StartedAt).Milliseconds()
	if runErr != nil {
		logError("chat_job_failed", runErr, fields)
	} else {
		logInfo("chat_job_finished", fields)
		go func() {
			titleCtx, cancel := context.WithTimeout(withRequestID(context.Background(), requestIDFromContext(ctx)), 20*time.Second)
			defer cancel()
			_ = a.maybeGenerateSessionTitle(titleCtx, sessionID, cfg)
		}()
	}
}

func (a *App) handleCreateChatJob(w http.ResponseWriter, r *http.Request) {
	var input model.ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, session, err := a.startChatJob(r.Context(), input)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"job": job, "session": session})
}

func (a *App) handleListChatJobs(w http.ResponseWriter, r *http.Request) {
	runningOnly := r.URL.Query().Get("running") != "0"
	jobs, err := a.store.ListChatJobs(r.URL.Query().Get("session_id"), runningOnly, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a *App) handleCancelChatJob(w http.ResponseWriter, r *http.Request) {
	job, err := a.cancelChatJob(r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"job": job})
}

func (a *App) handleChatJobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming is not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	streamChatJobEvents(r, w, flusher, a, jobID, after)
}

func streamChatJobEvents(r *http.Request, w http.ResponseWriter, flusher http.Flusher, a *App, jobID string, after int) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, events, err := a.store.ChatJobEventsAfter(jobID, after)
		if err != nil {
			_ = writeSSE(w, flusher, "error", map[string]string{"message": err.Error()})
			return
		}
		for _, event := range events {
			if err := writeSSE(w, flusher, event.Event, event.Data); err != nil {
				return
			}
			after = event.Seq
		}
		if job.Status != "running" {
			endPayload := map[string]any{"status": job.Status, "job": job, "request_id": job.RequestID}
			if session, ok := a.store.GetSession(job.SessionID); ok {
				endPayload["session"] = compactSessionToolEventDetails(session)
			}
			if job.Status == "failed" {
				_ = writeSSE(w, flusher, "error", chatStreamErrorPayload(job, llm.FirstNonEmptyString(job.Error, "chat job failed")))
				_ = writeSSE(w, flusher, "message_end", endPayload)
				return
			}
			_ = writeSSE(w, flusher, "message_end", endPayload)
			_ = writeSSE(w, flusher, "done", endPayload)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func chatStreamErrorPayload(job storepkg.ChatJob, rawMessage string) map[string]any {
	message := publicChatErrorMessage(rawMessage)
	return map[string]any{
		"type":       "error",
		"code":       chatErrorCode(rawMessage),
		"message":    message,
		"request_id": job.RequestID,
		"retryable":  isRetryableChatError(rawMessage),
		"status":     "failed",
	}
}

func publicChatErrorMessage(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "模型响应中断：未知错误。"
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "context canceled"):
		return "生成已中断。"
	case strings.Contains(lower, "client.timeout") || strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "模型响应中断：上游连接超时。"
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "connection reset"):
		return "模型响应中断：无法连接上游模型服务。"
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return "模型调用失败：模型供应商鉴权失败，请检查 API Key。"
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate limit"):
		return "模型调用失败：请求过于频繁或额度受限。"
	case strings.Contains(lower, "model api failed"):
		return "模型调用失败：上游模型服务返回错误。"
	default:
		return sanitizeLogText(text, 600)
	}
}

func chatErrorCode(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "UPSTREAM_TIMEOUT"
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "connection reset"):
		return "UPSTREAM_UNAVAILABLE"
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return "UPSTREAM_UNAUTHORIZED"
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate limit"):
		return "UPSTREAM_RATE_LIMITED"
	default:
		return "CHAT_STREAM_FAILED"
	}
}

func isRetryableChatError(raw string) bool {
	code := chatErrorCode(raw)
	return code == "UPSTREAM_TIMEOUT" || code == "UPSTREAM_UNAVAILABLE" || code == "UPSTREAM_RATE_LIMITED" || code == "CHAT_STREAM_FAILED"
}
