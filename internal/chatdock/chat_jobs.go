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

func (a *App) startChatJob(ctx context.Context, workspaceID string, input model.ChatRequest) (storepkg.ChatJob, *model.Session, error) {
	var session *model.Session
	var cfg model.ModelConfig
	var history []model.Message
	var err error
	if input.Regenerate {
		session, cfg, history, err = a.store.PrepareSessionRegeneration(workspaceID, input.SessionID)
	} else {
		input.Message = strings.TrimSpace(input.Message)
		if input.Message == "" && len(input.AttachmentIDs) == 0 {
			return storepkg.ChatJob{}, nil, fmt.Errorf("message is empty")
		}
		session, cfg, history, err = a.store.AppendUserMessageWithAttachments(workspaceID, input.SessionID, input.Message, input.AttachmentIDs)
	}
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	cfg, err = a.store.ResolveChatModelConfig(cfg, input.ProviderID, input.Model)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	if _, err := a.store.UpdateSessionModel(workspaceID, input.SessionID, cfg.ProviderID, cfg.Model); err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	requestID := requestIDFromContext(ctx)
	if requestID == "" {
		requestID = newRequestID()
	}
	job, err := a.store.CreateChatJob(workspaceID, input.SessionID, requestID)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	jobCtx, cancel := context.WithCancel(withRequestID(context.Background(), requestID))
	a.registerChatJobCancel(job.ID, cancel)
	logInfo("chat_job_started", logFields{"request_id": requestID, "job_id": job.ID, "session_id": input.SessionID, "provider_id": cfg.ProviderID, "model": cfg.Model})
	go a.runChatJob(jobCtx, workspaceID, job.ID, input.SessionID, cfg, history)
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

func (a *App) cancelChatJob(workspaceID string, jobID string) (storepkg.ChatJob, error) {
	jobID = strings.TrimSpace(jobID)
	a.jobMu.Lock()
	cancel := a.jobCancel[jobID]
	a.jobMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return a.store.InterruptChatJob(workspaceID, jobID, "用户已停止生成。")
}

func (a *App) runChatJob(ctx context.Context, workspaceID string, jobID string, sessionID string, cfg model.ModelConfig, history []model.Message) {
	defer a.unregisterChatJobCancel(jobID)
	defer a.clearChatJobGuidance(jobID)

	recorder := newAssistantOutputRecorder(a, workspaceID, sessionID, jobID)
	finalAnswer, runErr := a.completeWithRecordedTools(ctx, workspaceID, jobID, sessionID, cfg, history, recorder.emit)
	if err := recorder.flushDeltaEvent(true); err != nil && runErr == nil {
		runErr = err
	}
	recorder.useFinalAnswer(finalAnswer)
	status, runErr := chatJobCompletionStatus(ctx, runErr)
	if err := recorder.saveCheckpoint(true); err != nil {
		status = "failed"
		runErr = err
	}
	a.finishChatJob(ctx, workspaceID, sessionID, jobID, status, cfg, recorder, runErr)
}

func (a *App) handleCreateChatJob(w http.ResponseWriter, r *http.Request) {
	var input model.ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, session, err := a.startChatJob(r.Context(), a.workspaceIDFromRequest(r), input)
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
	jobs, err := a.store.ListChatJobs(a.workspaceIDFromRequest(r), r.URL.Query().Get("session_id"), runningOnly, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a *App) handleCancelChatJob(w http.ResponseWriter, r *http.Request) {
	job, err := a.cancelChatJob(a.workspaceIDFromRequest(r), r.PathValue("id"))
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
	streamChatJobEvents(r, w, flusher, a, a.workspaceIDFromRequest(r), jobID, after)
}

func streamChatJobEvents(r *http.Request, w http.ResponseWriter, flusher http.Flusher, a *App, workspaceID string, jobID string, after int) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, events, err := a.store.ChatJobEventsAfter(workspaceID, jobID, after)
		if err != nil {
			if writeErr := writeSSE(w, flusher, "error", map[string]string{"message": err.Error()}); writeErr != nil {
				return
			}
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
			if session, ok, err := a.store.GetSession(workspaceID, job.SessionID); err == nil && ok {
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
