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

func newChatJob(workspaceID string, sessionID string, requestID string, now time.Time) ChatJob {
	return ChatJob{Workspace: workspaceID, ID: model.NewID(), SessionID: strings.TrimSpace(sessionID), RequestID: strings.TrimSpace(requestID), Status: "running", StartedAt: now, UpdatedAt: now}
}

func insertChatJobWith(writer sqlWriter, job ChatJob) error {
	_, err := writer.Exec(`INSERT INTO chat_jobs(workspace_id, id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.Workspace, job.ID, job.SessionID, job.RequestID, job.Status, "", "", "", formatDBTime(job.StartedAt), "", formatDBTime(job.UpdatedAt))
	return err
}

func (s *Store) AddChatJobEvent(jobID string, event string, data any) (ChatJobEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID = strings.TrimSpace(jobID)
	event = strings.TrimSpace(event)
	if jobID == "" || event == "" {
		return ChatJobEvent{}, fmt.Errorf("job id and event are required")
	}
	if _, err := s.getChatJobByIDLocked(jobID); err != nil {
		return ChatJobEvent{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ChatJobEvent{}, err
	}
	defer func() { _ = tx.Rollback() }()
	created, err := addChatJobEventTx(tx, jobID, event, data, time.Now())
	if err != nil {
		return ChatJobEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return ChatJobEvent{}, err
	}
	return created, nil
}

func addChatJobEventTx(tx *sql.Tx, jobID string, event string, data any, now time.Time) (ChatJobEvent, error) {
	var seq int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM chat_job_events WHERE job_id = ?`, jobID).Scan(&seq); err != nil {
		return ChatJobEvent{}, err
	}
	if _, err := tx.Exec(`INSERT INTO chat_job_events(job_id, seq, event, data_json, created_at) VALUES(?, ?, ?, ?, ?)`, jobID, seq, event, compactJSONForDB(data), formatDBTime(now)); err != nil {
		return ChatJobEvent{}, err
	}
	result, err := tx.Exec(`UPDATE chat_jobs SET updated_at = ? WHERE id = ?`, formatDBTime(now), jobID)
	if err != nil {
		return ChatJobEvent{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return ChatJobEvent{}, err
	} else if affected != 1 {
		return ChatJobEvent{}, sql.ErrNoRows
	}
	return ChatJobEvent{JobID: jobID, Seq: seq, Event: event, Data: data, CreatedAt: now}, nil
}

func (s *Store) FinishChatJob(jobID string, status string, answer string, reasoning string, runErr error) (ChatJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID = strings.TrimSpace(jobID)
	existing, err := s.getChatJobByIDLocked(jobID)
	if err != nil {
		return ChatJob{}, err
	}
	status = normalizeChatJobFinishStatus(status)
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
	tx, err := s.db.Begin()
	if err != nil {
		return ChatJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(`UPDATE chat_jobs SET status = ?, answer = ?, reasoning = ?, error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, status, answer, strings.TrimSpace(reasoning), errorText, formatDBTime(now), formatDBTime(now), jobID)
	if err != nil {
		return ChatJob{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return ChatJob{}, err
	} else if affected != 1 {
		return ChatJob{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return ChatJob{}, err
	}
	return s.getChatJobByIDLocked(jobID)
}

func (s *Store) PruneChatJobStreamingEventsBefore(cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
DELETE FROM chat_job_events
WHERE event IN ('delta', 'reasoning_delta')
  AND job_id IN (
    SELECT id
    FROM chat_jobs
    WHERE status != 'running'
      AND finished_at != ''
      AND finished_at < ?
  )`, formatDBTime(cutoff))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func normalizeChatJobFinishStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "success", "failed", "interrupted":
		return strings.TrimSpace(status)
	case "":
		return "success"
	default:
		return "failed"
	}
}

func (s *Store) ListChatJobs(workspaceID string, sessionID string, runningOnly bool, limit int) ([]ChatJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT workspace_id, id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs WHERE workspace_id = ?`
	args := []any{workspaceID}
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

func (s *Store) GetChatJob(workspaceID string, jobID string) (ChatJob, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return ChatJob{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getChatJobForWorkspaceLocked(workspaceID, jobID)
}

func (s *Store) ChatJobEventsAfter(workspaceID string, jobID string, after int) (ChatJob, []ChatJobEvent, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return ChatJob{}, nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, err := s.getChatJobForWorkspaceLocked(workspaceID, jobID)
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

func (s *Store) InterruptChatJob(workspaceID string, jobID string, reason string) (ChatJob, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return ChatJob{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID = strings.TrimSpace(jobID)
	job, err := s.getChatJobForWorkspaceLocked(workspaceID, jobID)
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
	tx, err := s.db.Begin()
	if err != nil {
		return ChatJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(`UPDATE chat_jobs SET status = 'interrupted', error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(reason), formatDBTime(now), formatDBTime(now), jobID)
	if err != nil {
		return ChatJob{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return ChatJob{}, err
	} else if affected != 1 {
		return ChatJob{}, sql.ErrNoRows
	}
	if _, err := addChatJobEventTx(tx, jobID, "job_cancelled", map[string]any{"message": reason}, now); err != nil {
		return ChatJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return ChatJob{}, err
	}
	return s.getChatJobByIDLocked(jobID)
}

func (s *Store) getChatJobByIDLocked(jobID string) (ChatJob, error) {
	row := s.db.QueryRow(`SELECT workspace_id, id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs WHERE id = ?`, strings.TrimSpace(jobID))
	return scanSingleChatJob(row)
}

func (s *Store) getChatJobForWorkspaceLocked(workspaceID string, jobID string) (ChatJob, error) {
	row := s.db.QueryRow(`SELECT workspace_id, id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(jobID))
	return scanSingleChatJob(row)
}

func scanSingleChatJob(row *sql.Row) (ChatJob, error) {
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
			if err := json.Unmarshal([]byte(row.DataJSON), &data); err != nil {
				return nil, fmt.Errorf("decode chat job %s event %d data: %w", row.JobID, row.Seq, err)
			}
		}
		events = append(events, ChatJobEvent{JobID: row.JobID, Seq: row.Seq, Event: row.Event, Data: data, CreatedAt: parseDBTime(createdRaw)})
	}
	return events, rows.Err()
}
