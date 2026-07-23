package chatdock

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
)

type createSessionRequest struct {
	ProjectID string `json:"project_id,omitempty"`
}

func (a *App) handleListSessions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := sessionProjectFilterFromRequest(r)
	if strings.TrimSpace(query.Get("limit")) == "" && strings.TrimSpace(query.Get("cursor")) == "" {
		items, err := a.store.ListSessions(filter)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, items)
		return
	}
	limit, err := parseOptionalInt(query.Get("limit"), 30, 1, 100, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, nextCursor, hasMore, err := a.store.ListSessionPage(filter, query.Get("cursor"), limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"sessions": items, "next_cursor": nextCursor, "has_more": hasMore})
}

func (a *App) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var input createSessionRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	session, err := a.store.CreateSession(input.ProjectID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrProjectNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleGetSession(w http.ResponseWriter, r *http.Request) {
	session, ok, err := a.store.GetSession(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, model.ErrSessionNotFound)
		return
	}
	if r.URL.Query().Get("full") == "1" {
		writeJSONResponse(w, http.StatusOK, session)
		return
	}
	writeJSONResponse(w, http.StatusOK, compactSessionToolEventDetails(session))
}

func (a *App) handlePinSession(w http.ResponseWriter, r *http.Request) {
	var input model.PinSessionRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.store.PinSession(r.PathValue("id"), input.Pinned)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleUpdateSessionModel(w http.ResponseWriter, r *http.Request) {
	var input model.UpdateSessionModelRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.store.UpdateSessionModel(r.PathValue("id"), input.ProviderID, input.Model)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleRenameSession(w http.ResponseWriter, r *http.Request) {
	var input model.RenameSessionRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.store.RenameSession(r.PathValue("id"), input.Title)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrSessionNotFound) {
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
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleBranchSession(w http.ResponseWriter, r *http.Request) {
	var input model.BranchSessionRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.store.BranchSession(r.PathValue("id"), input.MessageIndex)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	var input model.EditMessageRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := a.store.EditUserMessageAndTruncate(r.PathValue("id"), input.MessageID, input.MessageIndex, input.Content)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, session)
}

func (a *App) handleExportSession(w http.ResponseWriter, r *http.Request) {
	session, ok, err := a.store.GetSession(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, model.ErrSessionNotFound)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" || format == "md" || format == "markdown" {
		filename := safeDownloadName(session.Title, session.ID) + ".md"
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		if _, err := io.WriteString(w, sessionToMarkdown(session)); err != nil {
			logError("session_export_write_failed", err, logFields{"request_id": requestIDFromRequest(r), "session_id": session.ID, "format": "markdown"})
		}
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
	ok, err := a.store.DeleteSession(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, model.ErrSessionNotFound)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func sessionProjectFilterFromRequest(r *http.Request) storepkg.SessionProjectFilter {
	value := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if value == "" {
		return storepkg.SessionProjectFilter{Mode: storepkg.SessionProjectFilterAll}
	}
	switch strings.ToLower(value) {
	case "null", "none", "empty":
		return storepkg.SessionProjectFilter{Mode: storepkg.SessionProjectFilterNoProject}
	default:
		return storepkg.SessionProjectFilter{Mode: storepkg.SessionProjectFilterByProject, ProjectID: value}
	}
}

func sessionToMarkdown(session *model.Session) string {
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
