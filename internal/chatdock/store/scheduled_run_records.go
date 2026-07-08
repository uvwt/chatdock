package store

import (
	"fmt"
	"strings"
	"time"

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

	query := `SELECT ` + scheduledTaskRunColumns() + ` FROM scheduled_task_runs WHERE workspace_id = ? AND task_id = ? ORDER BY started_at DESC LIMIT ?`
	rows, err := s.db.Query(query, s.workspaceCacheID, taskID, limit)
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

func (s *Store) loadScheduledTaskRunRecordsLocked() ([]model.ScheduledTaskRunRecord, error) {
	query := `SELECT ` + scheduledTaskRunColumns() + ` FROM scheduled_task_runs WHERE workspace_id = ? ORDER BY started_at DESC LIMIT 500`
	rows, err := s.db.Query(query, s.workspaceCacheID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []model.ScheduledTaskRunRecord{}
	for rows.Next() {
		record, err := scanScheduledTaskRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) saveScheduledTaskRunRecordsLocked(records []model.ScheduledTaskRunRecord) error {
	for _, record := range trimScheduledTaskRunRecords(records, 100, 500) {
		if err := upsertScheduledTaskRunTx(s.db, s.workspaceCacheID, normalizeScheduledRunRecordForDB(record)); err != nil {
			return err
		}
	}
	return s.touchWorkspaceLocked(s.workspaceCacheID, time.Now())
}

func sortScheduledTaskRunRecords(records []model.ScheduledTaskRunRecord) {
	// Kept for legacy tests/helpers; table queries sort in SQL.
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if records[j].StartedAt.After(records[i].StartedAt) {
				records[i], records[j] = records[j], records[i]
			}
		}
	}
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

func (s *Store) latestSuccessfulScheduledTaskRunLocked(taskID string) (model.ScheduledTaskRunRecord, bool, error) {
	taskID = strings.TrimSpace(taskID)
	query := `SELECT ` + scheduledTaskRunColumns() + ` FROM scheduled_task_runs WHERE workspace_id = ? AND task_id = ? AND status = 'success' AND output != '' ORDER BY started_at DESC LIMIT 1`
	row := s.db.QueryRow(query, s.workspaceCacheID, taskID)
	record, err := scanScheduledTaskRun(row)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return model.ScheduledTaskRunRecord{}, false, nil
		}
		return model.ScheduledTaskRunRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) appendScheduledTaskRunRecordLocked(record model.ScheduledTaskRunRecord) (model.ScheduledTaskRunRecord, error) {
	record = normalizeScheduledRunRecordForDB(record)
	if record.TaskID == "" {
		return model.ScheduledTaskRunRecord{}, fmt.Errorf("scheduled task id is empty")
	}
	if err := upsertScheduledTaskRunTx(s.db, s.workspaceCacheID, record); err != nil {
		return model.ScheduledTaskRunRecord{}, err
	}
	return record, nil
}
