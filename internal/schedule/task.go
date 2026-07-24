package schedule

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"chatdock/internal/model"
)

const (
	TypeOnce     = "once"
	TypeInterval = "interval"
	TypeCron     = "cron"
)

func SortTasks(tasks []model.ScheduledTask) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Pinned != tasks[j].Pinned {
			return tasks[i].Pinned
		}
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

func NormalizeInput(input model.ScheduledTaskRequest, previous *model.ScheduledTask, now time.Time) (model.ScheduledTask, error) {
	title, err := normalizeTitle(input.Title)
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
		scheduleType = TypeOnce
	}
	contextMode := NormalizeContextMode(input.ContextMode)
	task := model.ScheduledTask{Title: title, Prompt: prompt, Enabled: input.Enabled, ScheduleType: scheduleType, ContextMode: contextMode}
	switch scheduleType {
	case TypeOnce:
		runAt, err := parseTaskTime(input.RunAt)
		if err != nil {
			return model.ScheduledTask{}, err
		}
		task.RunAt = &runAt
		task.NextRunAt = runAt
	case TypeInterval:
		if input.IntervalMinutes <= 0 {
			return model.ScheduledTask{}, fmt.Errorf("interval_minutes must be greater than 0")
		}
		if input.IntervalMinutes > 525600 {
			return model.ScheduledTask{}, fmt.Errorf("interval_minutes is too large")
		}
		task.IntervalMinutes = input.IntervalMinutes
		task.NextRunAt = now.Add(time.Duration(input.IntervalMinutes) * time.Minute)
	case TypeCron:
		expressions, timezone, nextRunAt, err := normalizeCronSchedule(input.CronExpressions, input.Timezone, now)
		if err != nil {
			return model.ScheduledTask{}, err
		}
		task.CronExpressions = expressions
		task.Timezone = timezone
		task.NextRunAt = nextRunAt
	default:
		return model.ScheduledTask{}, fmt.Errorf("unsupported schedule_type: %s", scheduleType)
	}
	if previous != nil && !input.Reschedule && shouldPreserveNextRun(task, *previous) {
		task.NextRunAt = previous.NextRunAt
	}
	return task, nil
}

func shouldPreserveNextRun(next model.ScheduledTask, previous model.ScheduledTask) bool {
	if previous.NextRunAt.IsZero() {
		return false
	}
	// 保存内容和重新排期是两个动作：标题、提示词、启用状态、上下文模式变化都不应偷偷重置计时。
	// 只有调度计划本身变化，或调用方显式 reschedule=true，才重新计算 NextRunAt。
	if next.ScheduleType != previous.ScheduleType {
		return false
	}
	if next.IntervalMinutes != previous.IntervalMinutes || next.Timezone != previous.Timezone || !slices.Equal(next.CronExpressions, previous.CronExpressions) {
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

func NormalizeContextMode(value string) string {
	switch strings.TrimSpace(value) {
	case model.ScheduledTaskContextLastResult:
		return model.ScheduledTaskContextLastResult
	case model.ScheduledTaskContextSession:
		return model.ScheduledTaskContextSession
	default:
		return model.ScheduledTaskContextStateless
	}
}

func Advance(task model.ScheduledTask, now time.Time) (model.ScheduledTask, error) {
	switch task.ScheduleType {
	case TypeOnce:
		task.Enabled = false
	case TypeInterval:
		if task.IntervalMinutes > 0 {
			task.NextRunAt = now.Add(time.Duration(task.IntervalMinutes) * time.Minute)
		}
	case TypeCron:
		nextRunAt, err := nextCronRun(now, task.CronExpressions, task.Timezone)
		if err != nil {
			return task, err
		}
		task.NextRunAt = nextRunAt
	}
	return task, nil
}

func parseTaskTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("run_at is empty")
	}
	loc := location()
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

func normalizeTitle(title string) (string, error) {
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
	runes := []rune(title)
	if len(runes) > 80 {
		title = string(runes[:80])
	}
	return title, nil
}

func CloneTasks(tasks []model.ScheduledTask) []model.ScheduledTask {
	out := make([]model.ScheduledTask, len(tasks))
	copy(out, tasks)
	for i := range out {
		out[i].CronExpressions = append([]string(nil), tasks[i].CronExpressions...)
	}
	return out
}
