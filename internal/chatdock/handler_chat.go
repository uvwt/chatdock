package chatdock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (a *App) handleChat(w http.ResponseWriter, r *http.Request) {
	var input ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" && len(input.AttachmentIDs) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("message is empty"))
		return
	}

	_, cfg, history, err := a.store.AppendUserMessageWithAttachments(input.SessionID, input.Message, input.AttachmentIDs)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	answer, err := a.completeWithOptionalTools(r.Context(), input.SessionID, cfg, history)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	session, err := a.store.AppendAssistantMessage(input.SessionID, answer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, ChatResponse{Answer: answer, Session: session})
}

func (a *App) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var input ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	job, _, err := a.startChatJob(input)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
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
	_ = writeSSE(w, flusher, "job_started", job)
	streamChatJobEvents(r, w, flusher, a, job.ID, 0)
}

func (a *App) completeWithOptionalTools(ctx context.Context, sessionID string, cfg ModelConfig, history []Message) (string, error) {
	return a.completeWithRecordedTools(ctx, sessionID, cfg, history, nil)
}

func (a *App) streamWithOptionalTools(ctx context.Context, sessionID string, cfg ModelConfig, history []Message, emit func(string, any) error) (string, error) {
	return a.completeWithRecordedTools(ctx, sessionID, cfg, history, emit)
}

func isClientCanceled(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)
}
