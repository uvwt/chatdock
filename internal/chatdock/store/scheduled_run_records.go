package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
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

func sortScheduledTaskRunRecords(records []model.ScheduledTaskRunRecord) {
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})
}

func trimScheduledTaskRunRecords(records []model.ScheduledTaskRunRecord, perTaskLimit int, totalLimit int) []model.ScheduledTaskRunRecord {
	if perTaskLimit <= 0 {
		perTaskLimit = 100
	}
	if totalLimit <= 0 {
		totalLimit = 500
	}
	sortScheduledTaskRunRecords(records)
	counts := map[string]int{}
	out := make([]model.ScheduledTaskRunRecord, 0, len(records))
	for _, record := range records {
		if len(out) >= totalLimit {
			break
		}
		counts[record.TaskID]++
		if counts[record.TaskID] > perTaskLimit {
			continue
		}
		out = append(out, record)
	}
	return out
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

func (s *Store) appendScheduledTaskRunRecordLocked(workspaceID string, record model.ScheduledTaskRunRecord) (model.ScheduledTaskRunRecord, error) {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRunRecord{}, err
	}
	record = normalizeScheduledRunRecordForDB(record)
	if record.TaskID == "" {
		return model.ScheduledTaskRunRecord{}, fmt.Errorf("scheduled task id is empty")
	}
	if err := upsertScheduledTaskRunTx(s.db, workspaceID, record); err != nil {
		return model.ScheduledTaskRunRecord{}, err
	}
	return record, nil
}
