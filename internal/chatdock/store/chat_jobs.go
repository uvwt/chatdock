package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type ChatJob struct {
	Workspace  string     `json:"workspace"`
	ID         string     `json:"id"`
	SessionID  string     `json:"session_id"`
	RequestID  string     `json:"request_id,omitempty"`
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

func (s *Store) CreateChatJob(sessionID string, requestID string) (ChatJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(sessionID) == "" {
		return ChatJob{}, fmt.Errorf("session id is empty")
	}
	now := time.Now()
	job := ChatJob{Workspace: s.activePrompt, ID: model.NewID(), SessionID: strings.TrimSpace(sessionID), RequestID: strings.TrimSpace(requestID), Status: "running", StartedAt: now, UpdatedAt: now}
	_, err := s.db.Exec(`INSERT INTO chat_jobs(prompt, id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.Workspace, job.ID, job.SessionID, job.RequestID, job.Status, "", "", "", formatDBTime(job.StartedAt), "", formatDBTime(job.UpdatedAt))
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
	existing, _ := s.getChatJobLocked(jobID)
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
	if existing.Status == "interrupted" && status != "success" {
		status = "interrupted"
		if errorText == "" {
			errorText = existing.Error
		}
	}
	now := time.Now()
	_, err := s.db.Exec(`UPDATE chat_jobs SET status = ?, answer = ?, reasoning = ?, error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, status, answer, strings.TrimSpace(reasoning), errorText, formatDBTime(now), formatDBTime(now), jobID)
	if err != nil {
		return ChatJob{}, err
	}
	if status != "running" {
		if err := s.compactFinishedChatJobEventsLocked(jobID); err != nil {
			return ChatJob{}, err
		}
	}
	return s.getChatJobLocked(jobID)
}

func (s *Store) compactFinishedChatJobEventsLocked(jobID string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM chat_job_events WHERE job_id = ? AND event IN ('delta', 'reasoning_delta')`, jobID)
	return err
}

func (s *Store) ListChatJobs(sessionID string, runningOnly bool, limit int) ([]ChatJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT prompt, id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs WHERE prompt = ?`
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

func (s *Store) GetChatJob(jobID string) (ChatJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getChatJobLocked(jobID)
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

func (s *Store) InterruptChatJob(jobID string, reason string) (ChatJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID = strings.TrimSpace(jobID)
	job, err := s.getChatJobLocked(jobID)
	if err != nil {
		return ChatJob{}, err
	}
	if job.Status != "running" {
		return job, nil
	}
	if strings.TrimSpace(reason) == "" {
		reason = "用户已停止生成。"
	}
	now := time.Now()
	_, err = s.db.Exec(`UPDATE chat_jobs SET status = 'interrupted', error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(reason), formatDBTime(now), formatDBTime(now), jobID)
	if err != nil {
		return ChatJob{}, err
	}
	return s.getChatJobLocked(jobID)
}

func (s *Store) MarkRunningChatJobsInterrupted() error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE chat_jobs SET status = 'interrupted', error = 'ChatDock restarted before this job finished.', finished_at = ?, updated_at = ? WHERE status = 'running'`, formatDBTime(now), formatDBTime(now))
	return err
}

func (s *Store) getChatJobLocked(jobID string) (ChatJob, error) {
	row := s.db.QueryRow(`SELECT prompt, id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs WHERE id = ?`, strings.TrimSpace(jobID))
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
		if err := rows.Scan(&job.Workspace, &job.ID, &job.SessionID, &job.RequestID, &job.Status, &job.Answer, &job.Reasoning, &job.Error, &startedRaw, &finishedRaw, &updatedRaw); err != nil {
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
