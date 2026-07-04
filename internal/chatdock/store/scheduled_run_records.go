package store

import (
	"encoding/json"
	"fmt"
	"sort"
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

	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadScheduledTaskRunRecordsLocked()
	if err != nil {
		return model.ScheduledTaskRunRecordResponse{}, err
	}
	out := make([]model.ScheduledTaskRunRecord, 0, limit)
	for _, record := range records {
		if record.TaskID != taskID {
			continue
		}
		out = append(out, record)
		if len(out) >= limit {
			break
		}
	}
	return model.ScheduledTaskRunRecordResponse{Runs: out}, nil
}

func (s *Store) loadScheduledTaskRunRecordsLocked() ([]model.ScheduledTaskRunRecord, error) {
	raw, ok, err := s.getPromptRawLocked(s.activePrompt, scheduledTaskRunsKey)
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return []model.ScheduledTaskRunRecord{}, nil
	}
	var records []model.ScheduledTaskRunRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, fmt.Errorf("scheduled task runs config must be valid json: %w", err)
	}
	sortScheduledTaskRunRecords(records)
	return records, nil
}

func (s *Store) saveScheduledTaskRunRecordsLocked(records []model.ScheduledTaskRunRecord) error {
	sortScheduledTaskRunRecords(records)
	return s.setPromptJSONLocked(s.activePrompt, scheduledTaskRunsKey, trimScheduledTaskRunRecords(records, 100, 500))
}

func sortScheduledTaskRunRecords(records []model.ScheduledTaskRunRecord) {
	sort.SliceStable(records, func(i, j int) bool {
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

func (s *Store) latestSuccessfulScheduledTaskRunLocked(taskID string) (model.ScheduledTaskRunRecord, bool, error) {
	taskID = strings.TrimSpace(taskID)
	records, err := s.loadScheduledTaskRunRecordsLocked()
	if err != nil {
		return model.ScheduledTaskRunRecord{}, false, err
	}
	for _, record := range records {
		if record.TaskID == taskID && record.Status == "success" && strings.TrimSpace(record.Output) != "" {
			return record, true, nil
		}
	}
	return model.ScheduledTaskRunRecord{}, false, nil
}

func (s *Store) appendScheduledTaskRunRecordLocked(record model.ScheduledTaskRunRecord) (model.ScheduledTaskRunRecord, error) {
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		record.ID = model.NewID()
	}
	record.TaskID = strings.TrimSpace(record.TaskID)
	if record.TaskID == "" {
		return model.ScheduledTaskRunRecord{}, fmt.Errorf("scheduled task id is empty")
	}
	record.Prompt = strings.TrimSpace(record.Prompt)
	record.Output = strings.TrimSpace(record.Output)
	record.Error = strings.TrimSpace(record.Error)
	record.Status = strings.TrimSpace(record.Status)
	if record.Status == "" {
		record.Status = "success"
	}
	if record.StartedAt.IsZero() {
		record.StartedAt = time.Now()
	}
	if record.FinishedAt != nil && record.DurationMS <= 0 {
		record.DurationMS = record.FinishedAt.Sub(record.StartedAt).Milliseconds()
	}
	records, err := s.loadScheduledTaskRunRecordsLocked()
	if err != nil {
		return model.ScheduledTaskRunRecord{}, err
	}
	records = append(records, record)
	if err := s.saveScheduledTaskRunRecordsLocked(records); err != nil {
		return model.ScheduledTaskRunRecord{}, err
	}
	return record, nil
}
