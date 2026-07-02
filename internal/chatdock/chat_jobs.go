package chatdock

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ChatJob struct {
	Workspace  string     `json:"workspace"`
	ID         string     `json:"id"`
	SessionID  string     `json:"session_id"`
	Status     string     `json:"status"`
	Answer     string     `json:"answer,omitempty"`
	Reasoning  string     `json:"reasoning,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type ChatJobEvent struct {
	JobID     string    `json:"job_id"`
	Seq       int       `json:"seq"`
	Event     string    `json:"event"`
	Data      any       `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

type chatJobEventRow struct {
	JobID     string
	Seq       int
	Event     string
	DataJSON  string
	CreatedAt time.Time
}

func (s *Store) CreateChatJob(sessionID string) (ChatJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(sessionID) == "" {
		return ChatJob{}, fmt.Errorf("session id is empty")
	}
	now := time.Now()
	job := ChatJob{Workspace: s.activePrompt, ID: NewID(), SessionID: strings.TrimSpace(sessionID), Status: "running", StartedAt: now, UpdatedAt: now}
	_, err := s.db.Exec(`INSERT INTO chat_jobs(prompt, id, session_id, status, answer, reasoning, error, started_at, finished_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.Workspace, job.ID, job.SessionID, job.Status, "", "", "", formatDBTime(job.StartedAt), "", formatDBTime(job.UpdatedAt))
	if err != nil {
		return ChatJob{}, err
	}
	return job, nil
}

func (s *Store) AddChatJobEvent(jobID string, event string, data any) (ChatJobEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID = strings.TrimSpace(jobID)
	event = strings.TrimSpace(event)
	if jobID == "" || event == "" {
		return ChatJobEvent{}, fmt.Errorf("job id and event are required")
	}
	if _, err := s.getChatJobLocked(jobID); err != nil {
		return ChatJobEvent{}, err
	}
	var seq int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM chat_job_events WHERE job_id = ?`, jobID).Scan(&seq); err != nil {
		return ChatJobEvent{}, err
	}
	now := time.Now()
	raw := compactJSONForDB(data)
	_, err := s.db.Exec(`INSERT INTO chat_job_events(job_id, seq, event, data_json, created_at) VALUES(?, ?, ?, ?, ?)`, jobID, seq, event, raw, formatDBTime(now))
	if err != nil {
		return ChatJobEvent{}, err
	}
	_, _ = s.db.Exec(`UPDATE chat_jobs SET updated_at = ? WHERE id = ?`, formatDBTime(now), jobID)
	return ChatJobEvent{JobID: jobID, Seq: seq, Event: event, Data: data, CreatedAt: now}, nil
}

func (s *Store) FinishChatJob(jobID string, status string, answer string, reasoning string, runErr error) (ChatJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID = strings.TrimSpace(jobID)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "success"
	}
	if status != "success" && status != "failed" && status != "interrupted" {
		status = "failed"
	}
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}
	now := time.Now()
	_, err := s.db.Exec(`UPDATE chat_jobs SET status = ?, answer = ?, reasoning = ?, error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, status, answer, strings.TrimSpace(reasoning), errorText, formatDBTime(now), formatDBTime(now), jobID)
	if err != nil {
		return ChatJob{}, err
	}
	return s.getChatJobLocked(jobID)
}

func (s *Store) ListChatJobs(sessionID string, runningOnly bool, limit int) ([]ChatJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT prompt, id, session_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs WHERE prompt = ?`
	args := []any{s.activePrompt}
	if strings.TrimSpace(sessionID) != "" {
		query += ` AND session_id = ?`
		args = append(args, strings.TrimSpace(sessionID))
	}
	if runningOnly {
		query += ` AND status = 'running'`
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChatJobs(rows)
}

func (s *Store) ChatJobEventsAfter(jobID string, after int) (ChatJob, []ChatJobEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, err := s.getChatJobLocked(jobID)
	if err != nil {
		return ChatJob{}, nil, err
	}
	rows, err := s.db.Query(`SELECT job_id, seq, event, data_json, created_at FROM chat_job_events WHERE job_id = ? AND seq > ? ORDER BY seq ASC LIMIT 200`, strings.TrimSpace(jobID), after)
	if err != nil {
		return ChatJob{}, nil, err
	}
	defer rows.Close()
	events, err := scanChatJobEvents(rows)
	if err != nil {
		return ChatJob{}, nil, err
	}
	return job, events, nil
}

func (s *Store) MarkRunningChatJobsInterrupted() error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE chat_jobs SET status = 'interrupted', error = 'ChatDock restarted before this job finished.', finished_at = ?, updated_at = ? WHERE status = 'running'`, formatDBTime(now), formatDBTime(now))
	return err
}

func (s *Store) getChatJobLocked(jobID string) (ChatJob, error) {
	row := s.db.QueryRow(`SELECT prompt, id, session_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs WHERE id = ?`, strings.TrimSpace(jobID))
	jobs, err := scanChatJobs(rowRows{row: row})
	if err != nil {
		return ChatJob{}, err
	}
	if len(jobs) == 0 {
		return ChatJob{}, sql.ErrNoRows
	}
	return jobs[0], nil
}

type rowRows struct{ row *sql.Row }

func (r rowRows) Next() bool             { return r.row != nil }
func (r rowRows) Scan(dest ...any) error { err := r.row.Scan(dest...); r.row = nil; return err }
func (r rowRows) Err() error             { return nil }
func (r rowRows) Close() error           { return nil }

type chatJobRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

func scanChatJobs(rows chatJobRows) ([]ChatJob, error) {
	var jobs []ChatJob
	for rows.Next() {
		var job ChatJob
		var startedRaw, finishedRaw, updatedRaw string
		if err := rows.Scan(&job.Workspace, &job.ID, &job.SessionID, &job.Status, &job.Answer, &job.Reasoning, &job.Error, &startedRaw, &finishedRaw, &updatedRaw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return jobs, nil
			}
			return nil, err
		}
		job.StartedAt = parseDBTime(startedRaw)
		if finishedRaw != "" {
			finished := parseDBTime(finishedRaw)
			job.FinishedAt = &finished
		}
		job.UpdatedAt = parseDBTime(updatedRaw)
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func scanChatJobEvents(rows *sql.Rows) ([]ChatJobEvent, error) {
	var events []ChatJobEvent
	for rows.Next() {
		var row chatJobEventRow
		var createdRaw string
		if err := rows.Scan(&row.JobID, &row.Seq, &row.Event, &row.DataJSON, &createdRaw); err != nil {
			return nil, err
		}
		var data any
		if strings.TrimSpace(row.DataJSON) != "" {
			_ = json.Unmarshal([]byte(row.DataJSON), &data)
		}
		events = append(events, ChatJobEvent{JobID: row.JobID, Seq: row.Seq, Event: row.Event, Data: data, CreatedAt: parseDBTime(createdRaw)})
	}
	return events, rows.Err()
}

func (a *App) startChatJob(input ChatRequest) (ChatJob, *Session, error) {
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		return ChatJob{}, nil, fmt.Errorf("message is empty")
	}
	session, cfg, history, err := a.store.AppendUserMessage(input.SessionID, input.Message)
	if err != nil {
		return ChatJob{}, nil, err
	}
	job, err := a.store.CreateChatJob(input.SessionID)
	if err != nil {
		return ChatJob{}, nil, err
	}
	go a.runChatJob(job.ID, input.SessionID, cfg, history)
	return job, session, nil
}

func (a *App) runChatJob(jobID string, sessionID string, cfg ModelConfig, history []Message) {
	var reasoning strings.Builder
	emit := func(event string, value any) error {
		if event == "delta" {
			if delta, ok := value.(StreamDelta); ok && delta.ReasoningContent != "" {
				reasoning.WriteString(delta.ReasoningContent)
			}
		}
		_, err := a.store.AddChatJobEvent(jobID, event, value)
		return err
	}

	answer, runErr := a.completeWithRecordedTools(context.Background(), sessionID, cfg, history, emit)
	status := "success"
	if runErr != nil {
		status = "failed"
	} else if _, err := a.store.AppendAssistantMessageWithReasoning(sessionID, answer, reasoning.String()); err != nil {
		status = "failed"
		runErr = err
	}
	_, _ = a.store.FinishChatJob(jobID, status, answer, reasoning.String(), runErr)
}

func (a *App) handleCreateChatJob(w http.ResponseWriter, r *http.Request) {
	var input ChatRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, session, err := a.startChatJob(input)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrSessionNotFound) {
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
				_ = writeSSE(w, flusher, "error", map[string]string{"message": firstNonEmptyString(job.Error, "chat job failed")})
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
