package chatdock

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

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
