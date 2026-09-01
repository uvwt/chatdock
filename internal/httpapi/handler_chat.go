package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"chatdock/internal/model"
)

func (a *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var input model.ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" && len(input.AttachmentIDs) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("message is empty"))
		return
	}

	_, cfg, history, err := a.store.PrepareChat(input)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	answer, usedCfg, usage, err := a.completeWithOptionalTools(r.Context(), input.SessionID, cfg, history)
	if err != nil {
		a.persistSessionChatError("", input.SessionID, requestIDFromRequest(r), err)
		writeError(w, http.StatusBadGateway, err)
		return
	}

	session, err := a.store.AppendAssistantMessageWithReasoningAndUsage(input.SessionID, answer, "", usage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if renamed, titleErr := a.maybeGenerateSessionTitle(r.Context(), input.SessionID, usedCfg); titleErr != nil {
		logError("session_title_generation_failed", titleErr, logFields{"request_id": requestIDFromRequest(r), "session_id": input.SessionID})
	} else {
		session = renamed
	}
	writeJSONResponse(w, http.StatusOK, model.ChatResponse{Answer: answer, Session: session})
}

func (a *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var input model.ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	job, _, err := a.startChatJob(r.Context(), input)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, model.ErrSessionNotFound) {
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
	if err := writeSSE(w, flusher, "job_started", job); err != nil {
		return
	}
	streamChatJobEvents(r, w, flusher, a, job.ID, 0)
}

func (a *Server) completeWithOptionalTools(ctx context.Context, sessionID string, cfg model.ModelConfig, history []model.Message) (string, model.ModelConfig, *model.Usage, error) {
	fallbackCfg := a.resolveFallbackModelConfig(ctx, sessionID, cfg)
	return a.completeWithRecordedTools(ctx, "", sessionID, cfg, fallbackCfg, history, nil)
}

func isClientCanceled(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)
}
