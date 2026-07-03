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

func (a *App) startChatJob(input model.ChatRequest) (storepkg.ChatJob, *model.Session, error) {
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" && len(input.AttachmentIDs) == 0 {
		return storepkg.ChatJob{}, nil, fmt.Errorf("message is empty")
	}
	session, cfg, history, err := a.store.AppendUserMessageWithAttachments(input.SessionID, input.Message, input.AttachmentIDs)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	cfg, err = a.store.ResolveChatModelConfig(cfg, input.ProviderID, input.Model)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	job, err := a.store.CreateChatJob(input.SessionID)
	if err != nil {
		return storepkg.ChatJob{}, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.registerChatJobCancel(job.ID, cancel)
	go a.runChatJob(ctx, job.ID, input.SessionID, cfg, history)
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
	var reasoning strings.Builder
	emit := func(event string, value any) error {
		if event == "delta" {
			if delta, ok := value.(llm.StreamDelta); ok && delta.ReasoningContent != "" {
				reasoning.WriteString(delta.ReasoningContent)
			}
		}
		_, err := a.store.AddChatJobEvent(jobID, event, value)
		return err
	}

	answer, runErr := a.completeWithRecordedTools(ctx, sessionID, cfg, history, emit)
	status := "success"
	if isClientCanceled(ctx, runErr) {
		status = "interrupted"
		if runErr == nil {
			runErr = ctx.Err()
		}
	} else if runErr != nil {
		status = "failed"
	} else if _, err := a.store.AppendAssistantMessageWithReasoning(sessionID, answer, reasoning.String()); err != nil {
		status = "failed"
		runErr = err
	}
	_, _ = a.store.FinishChatJob(jobID, status, answer, reasoning.String(), runErr)
}

func (a *App) handleCreateChatJob(w http.ResponseWriter, r *http.Request) {
	var input model.ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, session, err := a.startChatJob(input)
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
			if job.Status == "failed" {
				_ = writeSSE(w, flusher, "error", map[string]string{"message": llm.FirstNonEmptyString(job.Error, "chat job failed")})
				return
			}
			if session, ok := a.store.GetSession(job.SessionID); ok {
				_ = writeSSE(w, flusher, "done", map[string]any{"session": session, "job": job})
			} else {
				_ = writeSSE(w, flusher, "done", map[string]any{"job": job})
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
