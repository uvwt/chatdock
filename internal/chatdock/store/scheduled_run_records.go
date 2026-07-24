package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/model"
)

const scheduledTaskRunsKey = "scheduled_task_runs"

func (s *Store) ListScheduledTaskRuns(taskID string, limit int) (model.ScheduledTaskRunRecordResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return model.ScheduledTaskRunRecordResponse{}, fmt.Errorf("scheduled task id is empty")
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	query := `SELECT id, task_id, task_title, task_prompt, output, status, error, manual,
		CASE WHEN EXISTS (
			SELECT 1
			FROM sessions
			WHERE sessions.id = scheduled_task_runs.session_id
		) THEN session_id ELSE '' END,
		started_at, finished_at, duration_ms,
		COALESCE((
			SELECT title
			FROM sessions
			WHERE sessions.id = scheduled_task_runs.session_id
		), '')
		FROM scheduled_task_runs
		WHERE task_id = ?
		ORDER BY started_at DESC LIMIT ?`
	rows, err := s.db.Query(query, taskID, limit)
	if err != nil {
		return model.ScheduledTaskRunRecordResponse{}, err
	}
	defer rows.Close()
	out := make([]model.ScheduledTaskRunRecord, 0, limit)
	for rows.Next() {
		record, err := scanScheduledTaskRunWithSessionTitle(rows)
		if err != nil {
			return model.ScheduledTaskRunRecordResponse{}, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return model.ScheduledTaskRunRecordResponse{}, err
	}
	return model.ScheduledTaskRunRecordResponse{Runs: out}, nil
}

func (s *Store) latestSuccessfulScheduledTaskRunLocked(taskID string) (model.ScheduledTaskRunRecord, bool, error) {
	taskID = strings.TrimSpace(taskID)
	query := `SELECT ` + scheduledTaskRunColumns() + ` FROM scheduled_task_runs WHERE task_id = ? AND status = 'success' AND output != '' ORDER BY started_at DESC LIMIT 1`
	row := s.db.QueryRow(query, taskID)
	record, err := scanScheduledTaskRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ScheduledTaskRunRecord{}, false, nil
		}
		return model.ScheduledTaskRunRecord{}, false, err
	}
	return record, true, nil
}
