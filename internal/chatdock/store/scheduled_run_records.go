package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"chatdock/internal/chatdock/model"
)

const scheduledTaskRunsKey = "scheduled_task_runs"

func (s *Store) ListScheduledTaskRuns(workspaceID string, taskID string, limit int) (model.ScheduledTaskRunRecordResponse, error) {
	workspaceID = strings.TrimSpace(workspaceID)
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
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRunRecordResponse{}, err
	}
	query := `SELECT ` + scheduledTaskRunColumns() + ` FROM scheduled_task_runs WHERE workspace_id = ? AND task_id = ? ORDER BY started_at DESC LIMIT ?`
	rows, err := s.db.Query(query, workspaceID, taskID, limit)
	if err != nil {
		return model.ScheduledTaskRunRecordResponse{}, err
	}
	defer rows.Close()
	out := make([]model.ScheduledTaskRunRecord, 0, limit)
	for rows.Next() {
		record, err := scanScheduledTaskRun(rows)
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

func (s *Store) latestSuccessfulScheduledTaskRunLocked(workspaceID string, taskID string) (model.ScheduledTaskRunRecord, bool, error) {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRunRecord{}, false, err
	}
	taskID = strings.TrimSpace(taskID)
	query := `SELECT ` + scheduledTaskRunColumns() + ` FROM scheduled_task_runs WHERE workspace_id = ? AND task_id = ? AND status = 'success' AND output != '' ORDER BY started_at DESC LIMIT 1`
	row := s.db.QueryRow(query, workspaceID, taskID)
	record, err := scanScheduledTaskRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ScheduledTaskRunRecord{}, false, nil
		}
		return model.ScheduledTaskRunRecord{}, false, err
	}
	return record, true, nil
}
