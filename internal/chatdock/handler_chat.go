package chatdock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"chatdock/internal/chatdock/model"
)

func (a *App) handleChat(w http.ResponseWriter, r *http.Request) {
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

	workspaceID := a.workspaceIDFromRequest(r)
	_, cfg, history, err := a.store.AppendUserMessageWithAttachments(workspaceID, input.SessionID, input.Message, input.AttachmentIDs)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	cfg, err = a.store.ResolveChatModelConfig(cfg, input.ProviderID, input.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.store.UpdateSessionModel(workspaceID, input.SessionID, cfg.ProviderID, cfg.Model); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	answer, err := a.completeWithOptionalTools(r.Context(), workspaceID, input.SessionID, cfg, history)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	session, err := a.store.AppendAssistantMessage(workspaceID, input.SessionID, answer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if renamed, titleErr := a.maybeGenerateSessionTitle(r.Context(), workspaceID, input.SessionID, cfg); titleErr != nil {
		logError("session_title_generation_failed", titleErr, logFields{"request_id": requestIDFromRequest(r), "session_id": input.SessionID})
	} else {
		session = renamed
	}
	writeJSONResponse(w, http.StatusOK, model.ChatResponse{Answer: answer, Session: session})
}

func (a *App) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var input model.ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	job, _, err := a.startChatJob(r.Context(), a.workspaceIDFromRequest(r), input)
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
	streamChatJobEvents(r, w, flusher, a, a.workspaceIDFromRequest(r), job.ID, 0)
}

func (a *App) completeWithOptionalTools(ctx context.Context, workspaceID string, sessionID string, cfg model.ModelConfig, history []model.Message) (string, error) {
	return a.completeWithRecordedTools(ctx, workspaceID, "", sessionID, cfg, history, nil)
}

func (a *App) streamWithOptionalTools(ctx context.Context, workspaceID string, sessionID string, cfg model.ModelConfig, history []model.Message, emit func(string, any) error) (string, error) {
	return a.completeWithRecordedTools(ctx, workspaceID, "", sessionID, cfg, history, emit)
}

func isClientCanceled(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)
}
