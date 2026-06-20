package chatdock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

func (a *App) handleGetMCPConfig(w http.ResponseWriter, r *http.Request) {
	content, err := a.store.GetMCPConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPConfigResponse{Content: content})
}

func (a *App) handleSaveMCPConfig(w http.ResponseWriter, r *http.Request) {
	var input SaveMCPConfigRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	content, err := a.store.SaveMCPConfig(input.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPConfigResponse{Content: content})
}

func (a *App) activeMCPConfig() (MCPConfig, error) {
	content, err := a.store.GetMCPConfig()
	if err != nil {
		return MCPConfig{}, err
	}
	return ParseMCPConfig(content)
}

func (a *App) handleListMCPTools(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tools, err := a.mcpClient.ListTools(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPToolsResponse{Tools: tools})
}

func (a *App) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	serverName := strings.TrimSpace(r.URL.Query().Get("server"))
	if serverName == "" {
		serverName = "agentdock"
	}
	tools, err := a.mcpClient.ListServerTools(r.Context(), cfg, serverName)
	if err != nil {
		writeJSONResponse(w, http.StatusBadGateway, map[string]any{"ok": false, "server": serverName, "error": err.Error()})
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "server": serverName, "tool_count": len(tools), "tools": tools})
}

func (a *App) handleCallMCPTool(w http.ResponseWriter, r *http.Request) {
	var input MCPToolCallRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("tool name is empty"))
		return
	}
	cfg, err := a.activeMCPConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.mcpClient.CallTool(r.Context(), cfg, input.Name, input.Arguments)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, MCPToolCallResponse{Name: input.Name, Result: result})
}

func (a *App) handleListSkills(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var input SaveSkillRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.CreateSkill(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleSkillRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	id := strings.Trim(path, "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, fmt.Errorf("skill not found"))
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input SaveSkillRequest
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.UpdateSkill(id, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	case http.MethodDelete:
		result, err := a.store.DeleteSkill(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (a *App) handleListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ListScheduledTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	var input ScheduledTaskRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := a.store.CreateScheduledTask(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleScheduledTaskRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/scheduled-tasks/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("scheduled task not found"))
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	if len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost {
		result, err := a.executeScheduledTask(r.Context(), a.store.ActivePrompt(), id, true)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, fmt.Errorf("scheduled task not found"))
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input ScheduledTaskRequest
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := a.store.UpdateScheduledTask(id, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	case http.MethodDelete:
		result, err := a.store.DeleteScheduledTask(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
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

func (a *App) handleSessionRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, ErrSessionNotFound)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			r.SetPathValue("id", id)
			a.handleGetSession(w, r)
		case http.MethodDelete:
			r.SetPathValue("id", id)
			a.handleDeleteSession(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		}
		return
	}
	if len(parts) == 2 && parts[1] == "rename" && r.Method == http.MethodPost {
		r.SetPathValue("id", id)
		a.handleRenameSession(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "pin" && r.Method == http.MethodPost {
		r.SetPathValue("id", id)
		a.handlePinSession(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "clone" && r.Method == http.MethodPost {
		r.SetPathValue("id", id)
		a.handleCloneSession(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "export" && r.Method == http.MethodGet {
		r.SetPathValue("id", id)
		a.handleExportSession(w, r)
		return
	}
	writeError(w, http.StatusNotFound, ErrSessionNotFound)
}

func (a *App) handlePinSession(w http.ResponseWriter, r *http.Request) {
	var input PinSessionRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.store.PinSession(r.PathValue("id"), input.Pinned)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleRenameSession(w http.ResponseWriter, r *http.Request) {
	var input RenameSessionRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.store.RenameSession(r.PathValue("id"), input.Title)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleCloneSession(w http.ResponseWriter, r *http.Request) {
	session, err := a.store.CloneSession(r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleExportSession(w http.ResponseWriter, r *http.Request) {
	session, ok := a.store.GetSession(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, ErrSessionNotFound)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" || format == "md" || format == "markdown" {
		filename := safeDownloadName(session.Title, session.ID) + ".md"
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = io.WriteString(w, sessionToMarkdown(session))
		return
	}
	if format == "json" {
		filename := safeDownloadName(session.Title, session.ID) + ".json"
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		writeJSONResponse(w, http.StatusOK, session)
		return
	}
	writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported export format: %s", format))
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

	answer, err := a.streamWithOptionalTools(r.Context(), input.SessionID, cfg, history, func(event string, value any) error {
		return writeSSE(w, flusher, event, value)
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

func (a *App) completeWithOptionalTools(ctx context.Context, sessionID string, cfg ModelConfig, history []Message) (string, error) {
	return a.completeWithRecordedTools(ctx, sessionID, cfg, history, nil)
}

func (a *App) streamWithOptionalTools(ctx context.Context, sessionID string, cfg ModelConfig, history []Message, emit func(string, any) error) (string, error) {
	return a.completeWithRecordedTools(ctx, sessionID, cfg, history, emit)
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

func sessionToMarkdown(session *Session) string {
	var b strings.Builder
	title := strings.TrimSpace(session.Title)
	if title == "" {
		title = session.ID
	}
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString("- ID: ")
	b.WriteString(session.ID)
	b.WriteString("\n- Created: ")
	b.WriteString(session.CreatedAt.Format(time.RFC3339))
	b.WriteString("\n- Updated: ")
	b.WriteString(session.UpdatedAt.Format(time.RFC3339))
	b.WriteString("\n\n")
	for _, msg := range session.Messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "message"
		}
		b.WriteString("## ")
		b.WriteString(strings.ToUpper(role[:1]))
		if len(role) > 1 {
			b.WriteString(role[1:])
		}
		if !msg.CreatedAt.IsZero() {
			b.WriteString(" · ")
			b.WriteString(msg.CreatedAt.Format(time.RFC3339))
		}
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(msg.Content))
		b.WriteString("\n\n")
	}
	return b.String()
}

var downloadNameRegexp = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeDownloadName(title string, fallback string) string {
	name := strings.Trim(downloadNameRegexp.ReplaceAllString(strings.TrimSpace(title), "-"), "-._")
	if name == "" {
		name = strings.Trim(downloadNameRegexp.ReplaceAllString(fallback, "-"), "-._")
	}
	if name == "" {
		name = "chatdock-session"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}
