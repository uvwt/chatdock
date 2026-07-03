package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"chatdock/internal/chatdock/model"
)

func (s *Store) loadScheduledTasksLocked() ([]model.ScheduledTask, error) {
	raw, ok, err := s.getPromptRawLocked(s.activePrompt, "scheduled_tasks")
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		tasks := []model.ScheduledTask{}
		return tasks, s.saveScheduledTasksLocked(tasks)
	}
	var tasks []model.ScheduledTask
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		return nil, fmt.Errorf("scheduled tasks config must be valid json: %w", err)
	}
	for i := range tasks {
		if tasks[i].Running && time.Since(tasks[i].UpdatedAt) > 2*time.Hour {
			tasks[i].Running = false
			tasks[i].LastError = "上次运行异常中断，已自动恢复为可运行状态"
		}
	}
	sortScheduledTasks(tasks)
	return tasks, nil
}

func (s *Store) saveScheduledTasksLocked(tasks []model.ScheduledTask) error {
	sortScheduledTasks(tasks)
	return s.setPromptJSONLocked(s.activePrompt, "scheduled_tasks", tasks)
}

func sortScheduledTasks(tasks []model.ScheduledTask) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Enabled != tasks[j].Enabled {
			return tasks[i].Enabled
		}
		if tasks[i].NextRunAt.IsZero() != tasks[j].NextRunAt.IsZero() {
			return !tasks[i].NextRunAt.IsZero()
		}
		if !tasks[i].NextRunAt.Equal(tasks[j].NextRunAt) {
			return tasks[i].NextRunAt.Before(tasks[j].NextRunAt)
		}
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})
}

func normalizeScheduledTaskInput(input model.ScheduledTaskRequest, previous *model.ScheduledTask, now time.Time) (model.ScheduledTask, error) {
	title, err := normalizeScheduledTaskTitle(input.Title)
	if err != nil {
		return model.ScheduledTask{}, err
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return model.ScheduledTask{}, fmt.Errorf("scheduled task prompt is empty")
	}
	if len([]byte(prompt)) > 40000 {
		return model.ScheduledTask{}, fmt.Errorf("scheduled task prompt is too large")
	}
	scheduleType := strings.ToLower(strings.TrimSpace(input.ScheduleType))
	if scheduleType == "" {
		scheduleType = scheduleTypeOnce
	}
	task := model.ScheduledTask{Title: title, Prompt: prompt, Enabled: input.Enabled, ScheduleType: scheduleType}
	switch scheduleType {
	case scheduleTypeOnce:
		runAt, err := parseTaskTime(input.RunAt)
		if err != nil {
			return model.ScheduledTask{}, err
		}
		task.RunAt = &runAt
		task.NextRunAt = runAt
	case scheduleTypeDaily:
		tod, err := normalizeTimeOfDay(input.TimeOfDay)
		if err != nil {
			return model.ScheduledTask{}, err
		}
		task.TimeOfDay = tod
		task.NextRunAt = nextDailyRun(now, tod)
	case scheduleTypeInterval:
		if input.IntervalMinutes <= 0 {
			return model.ScheduledTask{}, fmt.Errorf("interval_minutes must be greater than 0")
		}
		if input.IntervalMinutes > 525600 {
			return model.ScheduledTask{}, fmt.Errorf("interval_minutes is too large")
		}
		task.IntervalMinutes = input.IntervalMinutes
		task.NextRunAt = now.Add(time.Duration(input.IntervalMinutes) * time.Minute)
	default:
		return model.ScheduledTask{}, fmt.Errorf("unsupported schedule_type: %s", scheduleType)
	}
	if previous != nil && sameScheduledTaskPlan(task, *previous) && !previous.NextRunAt.IsZero() {
		task.NextRunAt = previous.NextRunAt
	}
	return task, nil
}

func sameScheduledTaskPlan(next model.ScheduledTask, previous model.ScheduledTask) bool {
	if next.ScheduleType != previous.ScheduleType || next.Prompt != previous.Prompt || next.Title != previous.Title {
		return false
	}
	if next.IntervalMinutes != previous.IntervalMinutes || next.TimeOfDay != previous.TimeOfDay {
		return false
	}
	if (next.RunAt == nil) != (previous.RunAt == nil) {
		return false
	}
	if next.RunAt != nil && previous.RunAt != nil && !next.RunAt.Equal(*previous.RunAt) {
		return false
	}
	return true
}

func advanceScheduledTask(task model.ScheduledTask, now time.Time) model.ScheduledTask {
	switch task.ScheduleType {
	case scheduleTypeOnce:
		task.Enabled = false
	case scheduleTypeDaily:
		task.NextRunAt = nextDailyRun(now.Add(time.Minute), task.TimeOfDay)
	case scheduleTypeInterval:
		if task.IntervalMinutes > 0 {
			task.NextRunAt = now.Add(time.Duration(task.IntervalMinutes) * time.Minute)
		}
	}
	return task
}

func parseTaskTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("run_at is empty")
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04"}
	var lastErr error
	for _, layout := range layouts {
		if layout == time.RFC3339 {
			parsed, err := time.Parse(layout, value)
			if err == nil {
				return parsed.Local(), nil
			}
			lastErr = err
			continue
		}
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("run_at must be RFC3339 or local datetime: %w", lastErr)
}

func normalizeTimeOfDay(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return "", fmt.Errorf("time_of_day must use HH:MM format")
	}
	return parsed.Format("15:04"), nil
}

func nextDailyRun(now time.Time, timeOfDay string) time.Time {
	parsed, _ := time.Parse("15:04", timeOfDay)
	next := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func normalizeScheduledTaskTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("scheduled task title is empty")
	}
	if !utf8.ValidString(title) {
		return "", fmt.Errorf("scheduled task title is invalid")
	}
	for _, r := range title {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("scheduled task title contains control characters")
		}
	}
	return limitRunes(title, 80), nil
}

func cloneScheduledTasks(tasks []model.ScheduledTask) []model.ScheduledTask {
	out := make([]model.ScheduledTask, len(tasks))
	copy(out, tasks)
	return out
}
