package chatdock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, map[string]any{
		"ok":   true,
		"name": "ChatDock",
		"time": time.Now(),
	})
}

func (a *App) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, ToPublicModelConfig(a.store.GetModelConfig()))
}

func (a *App) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var input ModelConfig
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	cfg, err := a.store.SaveModelConfig(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, ToPublicModelConfig(cfg))
}

func (a *App) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListPrompts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	var input CreatePromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.CreatePrompt(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleSelectPrompt(w http.ResponseWriter, r *http.Request) {
	var input SelectPromptRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.SelectPrompt(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, http.StatusOK, a.store.ListSessions())
}

func (a *App) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	session, err := a.store.CreateSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleGetSession(w http.ResponseWriter, r *http.Request) {
	session, ok := a.store.GetSession(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, ErrSessionNotFound)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if ok := a.store.DeleteSession(r.PathValue("id")); !ok {
		writeError(w, http.StatusNotFound, ErrSessionNotFound)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleChat(w http.ResponseWriter, r *http.Request) {
	var input ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("message is empty"))
		return
	}

	_, cfg, history, err := a.store.AppendUserMessage(input.SessionID, input.Message)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	answer, err := a.client.Complete(r.Context(), cfg, history)
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

	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("message is empty"))
		return
	}

	_, cfg, history, err := a.store.AppendUserMessage(input.SessionID, input.Message)
	if err != nil {
		status := http.StatusInternalServerError
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

	answer, err := a.client.Stream(r.Context(), cfg, history, func(delta StreamDelta) error {
		return writeSSE(w, flusher, "delta", delta)
	})
	if err != nil {
		if isClientCanceled(r.Context(), err) && strings.TrimSpace(answer) != "" {
			_, _ = a.store.AppendAssistantMessage(input.SessionID, strings.TrimSpace(answer)+"\n\n【已中断】")
		}
		_ = writeSSE(w, flusher, "error", map[string]string{"message": err.Error()})
		return
	}

	session, err := a.store.AppendAssistantMessage(input.SessionID, answer)
	if err != nil {
		_ = writeSSE(w, flusher, "error", map[string]string{"message": err.Error()})
		return
	}
	_ = writeSSE(w, flusher, "done", map[string]any{"session": session})
}

func isClientCanceled(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(out)
}

func writeJSONResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSONResponse(w, status, map[string]any{"error": err.Error()})
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
