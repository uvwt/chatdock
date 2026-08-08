package httpapi

import (
	"chatdock/internal/chatoutput"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chatdock/internal/llm"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

func (a *Server) startChatJob(ctx context.Context, input model.ChatRequest) (storepkg.ChatJob, *model.Session, error) {
	if err := a.reserveChatJob(); err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	launched := false
	defer func() {
		if !launched {
			a.backgroundWG.Done()
		}
	}()

	requestID := requestIDFromContext(ctx)
	if requestID == "" {
		requestID = newRequestID()
	}
	job, session, cfg, history, err := a.store.PrepareChatJob(input, requestID)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	fallbackCfg := a.resolveFallbackModelConfig(withRequestID(ctx, requestID), input.SessionID, cfg)
	jobCtx, cancel := context.WithCancel(withRequestID(a.lifecycleCtx, requestID))
	a.registerChatJobCancel(job.ID, cancel)
	logInfo("chat_job_started", logFields{"request_id": requestID, "job_id": job.ID, "session_id": input.SessionID, "provider_id": cfg.ProviderID, "model": cfg.Model})
	launched = true
	go func() {
		defer a.backgroundWG.Done()
		a.runChatJob(jobCtx, job.ID, input.SessionID, cfg, fallbackCfg, history)
	}()
	return job, session, nil
}

func (a *Server) reserveChatJob() error {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	if a.closing {
		return errAppShuttingDown
	}
	a.backgroundWG.Add(1)
	return nil
}

func (a *Server) registerChatJobCancel(jobID string, cancel context.CancelFunc) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	// 只有成功 reserveChatJob 的调用方能到这里。即使停服刚开始，
	// 该任务也属于已接纳工作；它会继承已取消的 lifecycle context 并被 Close 等待。
	a.jobCancel[strings.TrimSpace(jobID)] = cancel
}

func (a *Server) unregisterChatJobCancel(jobID string) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	delete(a.jobCancel, strings.TrimSpace(jobID))
}

func (a *Server) cancelChatJob(jobID string) (storepkg.ChatJob, error) {
	jobID = strings.TrimSpace(jobID)
	a.jobMu.Lock()
	cancel := a.jobCancel[jobID]
	a.jobMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return a.store.InterruptChatJob(jobID, "用户已停止生成。")
}

func (a *Server) runChatJob(ctx context.Context, jobID string, sessionID string, cfg model.ModelConfig, fallbackCfg *model.ModelConfig, history []model.Message) {
	defer a.unregisterChatJobCancel(jobID)
	defer a.clearChatJobGuidance(jobID)

	recorder := chatoutput.NewRecorder(a.store, sessionID, jobID)
	finalAnswer, usedCfg, runErr := a.completeWithRecordedTools(ctx, jobID, sessionID, cfg, fallbackCfg, history, recorder.Emit)
	if err := recorder.FlushDeltaEvent(true); err != nil && runErr == nil {
		runErr = err
	}
	recorder.UseFinalAnswer(finalAnswer)
	status, runErr := chatJobCompletionStatus(ctx, runErr)
	if status == "failed" && runErr != nil {
		recorder.SetError(newMessageError(requestIDFromContext(ctx), runErr.Error()))
	}
	if err := recorder.SaveCheckpoint(true); err != nil {
		status = "failed"
		runErr = err
	}
	a.finishChatJob(ctx, sessionID, jobID, status, usedCfg, recorder, runErr)
}

func newMessageError(requestID string, rawMessage string) model.MessageError {
	rawMessage = strings.TrimSpace(rawMessage)
	return model.MessageError{
		Message:   publicChatErrorMessage(rawMessage),
		Raw:       rawMessage,
		Code:      chatErrorCode(rawMessage),
		RequestID: strings.TrimSpace(requestID),
		Retryable: isRetryableChatError(rawMessage),
	}
}

func (a *Server) persistSessionChatError(_ string, sessionID string, requestID string, runErr error) {
	if runErr == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	messageError := newMessageError(requestID, runErr.Error())
	if _, _, err := a.store.UpsertAssistantMessageCheckpoint(sessionID, "", "", "", nil, nil, &messageError); err != nil && !errors.Is(err, model.ErrSessionNotFound) {
		logError("chat_error_persist_failed", err, logFields{"request_id": requestID, "session_id": sessionID})
	}
}

func (a *Server) handleCreateChatJob(w http.ResponseWriter, r *http.Request) {
	var input model.ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, session, err := a.startChatJob(r.Context(), input)
	if err != nil {
		writeError(w, chatPreparationHTTPStatus(err), err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"job": job, "session": session})
}

func (a *Server) handleListChatJobs(w http.ResponseWriter, r *http.Request) {
	runningOnly := r.URL.Query().Get("running") != "0"
	jobs, err := a.store.ListChatJobs(r.URL.Query().Get("session_id"), runningOnly, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a *Server) handleCancelChatJob(w http.ResponseWriter, r *http.Request) {
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

func (a *Server) handleChatJobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	after, err := chatJobEventCursor(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
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

func streamChatJobEvents(r *http.Request, w http.ResponseWriter, flusher http.Flusher, a *Server, jobID string, after int) {
	const heartbeatInterval = 15 * time.Second

	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	lastWriteAt := time.Now()
	for {
		job, events, err := a.store.ChatJobEventsAfter(jobID, after)
		if err != nil {
			if writeErr := writeSSE(w, flusher, "error", map[string]string{"message": err.Error()}); writeErr != nil {
				return
			}
			return
		}
		for _, event := range events {
			if err := writeSSEWithID(w, flusher, event.Seq, event.Event, event.Data); err != nil {
				return
			}
			after = event.Seq
			lastWriteAt = time.Now()
		}
		if job.Status != "running" {
			endPayload := map[string]any{"status": job.Status, "job": job, "request_id": job.RequestID}
			if session, ok, err := a.store.GetSession(job.SessionID); err == nil && ok {
				endPayload["session"] = compactSessionToolEventDetails(session)
			}
			if job.Status == "failed" {
				if err := writeSSE(w, flusher, "error", chatStreamErrorPayload(job, llm.FirstNonEmptyString(job.Error, "chat job failed"))); err != nil {
					return
				}
				if err := writeSSE(w, flusher, "message_end", endPayload); err != nil {
					return
				}
				return
			}
			if err := writeSSE(w, flusher, "message_end", endPayload); err != nil {
				return
			}
			if err := writeSSE(w, flusher, "done", endPayload); err != nil {
				return
			}
			return
		}
		if time.Since(lastWriteAt) >= heartbeatInterval {
			if err := writeSSEHeartbeat(w, flusher); err != nil {
				return
			}
			lastWriteAt = time.Now()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func chatStreamErrorPayload(job storepkg.ChatJob, rawMessage string) map[string]any {
	messageError := newMessageError(job.RequestID, rawMessage)
	return map[string]any{
		"type":       "error",
		"code":       messageError.Code,
		"message":    messageError.Message,
		"raw":        messageError.Raw,
		"request_id": messageError.RequestID,
		"retryable":  messageError.Retryable,
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
	case strings.Contains(lower, "400") || strings.Contains(lower, "invalidparameter") || strings.Contains(lower, "invalid request body"):
		return "模型调用失败：请求参数不受当前模型供应商支持。"
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
	case strings.Contains(lower, "400") || strings.Contains(lower, "invalidparameter") || strings.Contains(lower, "invalid request body"):
		return "UPSTREAM_BAD_REQUEST"
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
