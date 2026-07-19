package store

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"chatdock/internal/chatdock/model"
)

func (s *Store) loadScheduledTasksForWorkspaceLocked(workspaceID string) ([]model.ScheduledTask, error) {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return nil, err
	}
	tasks, err := loadScheduledTasksForWorkspaceLocked(s.db, workspaceID)
	if err != nil {
		return nil, err
	}
	changed := false
	now := time.Now()
	for i := range tasks {
		tasks[i].ContextMode = normalizeScheduledTaskContextMode(tasks[i].ContextMode)
		if tasks[i].Running && time.Since(tasks[i].UpdatedAt) > 2*time.Hour {
			tasks[i].Running = false
			tasks[i].LastError = "上次运行异常中断，已自动恢复为可运行状态"
			changed = true
		}
		if repairScheduledTaskNextRun(&tasks[i], now) {
			changed = true
		}
	}
	sortScheduledTasks(tasks)
	if changed {
		if err := s.saveScheduledTasksForWorkspaceLocked(workspaceID, tasks); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (s *Store) saveScheduledTasksForWorkspaceLocked(workspaceID string, tasks []model.ScheduledTask) error {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	sortScheduledTasks(tasks)
	keep := map[string]bool{}
	for _, task := range tasks {
		task = normalizeScheduledTaskForDB(task)
		keep[task.ID] = true
		if err := upsertScheduledTaskTx(tx, workspaceID, task); err != nil {
			return err
		}
	}
	if err := deleteScheduledTasksExceptWorkspaceLocked(tx, workspaceID, keep); err != nil {
		return err
	}
	if err := touchWorkspace(tx, workspaceID, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
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
	contextMode := normalizeScheduledTaskContextMode(input.ContextMode)
	task := model.ScheduledTask{Title: title, Prompt: prompt, Enabled: input.Enabled, ScheduleType: scheduleType, ContextMode: contextMode}
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
	if previous != nil && !input.Reschedule && shouldPreserveScheduledTaskNextRun(task, *previous) {
		task.NextRunAt = previous.NextRunAt
	}
	return task, nil
}

func shouldPreserveScheduledTaskNextRun(next model.ScheduledTask, previous model.ScheduledTask) bool {
	if previous.NextRunAt.IsZero() {
		return false
	}
	// 保存内容和重新排期是两个动作：标题、提示词、启用状态、上下文模式变化都不应偷偷重置计时。
	// 只有调度计划本身变化，或调用方显式 reschedule=true，才重新计算 NextRunAt。
	if next.ScheduleType != previous.ScheduleType {
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

func normalizeScheduledTaskContextMode(value string) string {
	switch strings.TrimSpace(value) {
	case model.ScheduledTaskContextLastResult:
		return model.ScheduledTaskContextLastResult
	case model.ScheduledTaskContextSession:
		return model.ScheduledTaskContextSession
	default:
		return model.ScheduledTaskContextStateless
	}
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
	loc := scheduleLocation()
	layouts := []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04"}
	var lastErr error
	for _, layout := range layouts {
		if layout == time.RFC3339 {
			parsed, err := time.Parse(layout, value)
			if err == nil {
				return parsed.In(loc), nil
			}
			lastErr = err
			continue
		}
		parsed, err := time.ParseInLocation(layout, value, loc)
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
	loc := scheduleLocation()
	localNow := now.In(loc)
	parsed, _ := time.Parse("15:04", timeOfDay)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)
	if !next.After(localNow) {
		next = time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, parsed.Hour(), parsed.Minute(), 0, 0, loc)
	}
	return next
}

func scheduleLocation() *time.Location {
	name := strings.TrimSpace(os.Getenv("CHATDOCK_TIMEZONE"))
	if name == "" {
		name = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(name)
	if err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*60*60)
}

func repairScheduledTaskNextRun(task *model.ScheduledTask, now time.Time) bool {
	if task == nil || task.ScheduleType != scheduleTypeDaily || strings.TrimSpace(task.TimeOfDay) == "" {
		return false
	}
	if _, err := normalizeTimeOfDay(task.TimeOfDay); err != nil {
		return false
	}
	loc := scheduleLocation()
	localNext := task.NextRunAt.In(loc)
	if !task.NextRunAt.IsZero() && localNext.Format("15:04") == task.TimeOfDay {
		return false
	}
	task.NextRunAt = nextDailyRun(now, task.TimeOfDay)
	return true
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
