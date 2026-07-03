package chatdock

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	scheduleTypeOnce     = "once"
	scheduleTypeDaily    = "daily"
	scheduleTypeInterval = "interval"
)

func (s *Store) ListScheduledTasks() (ScheduledTaskResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return ScheduledTaskResponse{}, err
	}
	return ScheduledTaskResponse{Tasks: cloneScheduledTasks(tasks)}, nil
}

func (s *Store) CreateScheduledTask(input ScheduledTaskRequest) (ScheduledTaskResponse, error) {
	now := time.Now()
	next, err := normalizeScheduledTaskInput(input, nil, now)
	if err != nil {
		return ScheduledTaskResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return ScheduledTaskResponse{}, err
	}
	for _, task := range tasks {
		if strings.EqualFold(task.Title, next.Title) {
			return ScheduledTaskResponse{}, fmt.Errorf("scheduled task already exists: %s", next.Title)
		}
	}
	next.ID = NewID()
	next.CreatedAt = now
	next.UpdatedAt = now
	tasks = append(tasks, next)
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return ScheduledTaskResponse{}, err
	}
	return ScheduledTaskResponse{Tasks: cloneScheduledTasks(tasks)}, nil
}

func (s *Store) UpdateScheduledTask(id string, input ScheduledTaskRequest) (ScheduledTaskResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ScheduledTaskResponse{}, fmt.Errorf("scheduled task id is empty")
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return ScheduledTaskResponse{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ScheduledTaskResponse{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	for i, task := range tasks {
		if i != index && strings.EqualFold(task.Title, input.Title) {
			return ScheduledTaskResponse{}, fmt.Errorf("scheduled task already exists: %s", input.Title)
		}
	}
	next, err := normalizeScheduledTaskInput(input, &tasks[index], now)
	if err != nil {
		return ScheduledTaskResponse{}, err
	}
	next.ID = tasks[index].ID
	next.SessionID = tasks[index].SessionID
	next.Running = tasks[index].Running
	next.LastRunAt = tasks[index].LastRunAt
	next.LastStatus = tasks[index].LastStatus
	next.LastError = tasks[index].LastError
	next.CreatedAt = tasks[index].CreatedAt
	if next.CreatedAt.IsZero() {
		next.CreatedAt = now
	}
	next.UpdatedAt = now
	tasks[index] = next
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return ScheduledTaskResponse{}, err
	}
	return ScheduledTaskResponse{Tasks: cloneScheduledTasks(tasks)}, nil
}

func (s *Store) DeleteScheduledTask(id string) (ScheduledTaskResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ScheduledTaskResponse{}, fmt.Errorf("scheduled task id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return ScheduledTaskResponse{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ScheduledTaskResponse{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	tasks = append(tasks[:index], tasks[index+1:]...)
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return ScheduledTaskResponse{}, err
	}
	return ScheduledTaskResponse{Tasks: cloneScheduledTasks(tasks)}, nil
}

func (s *Store) DueScheduledTasks(now time.Time) ([]ScheduledTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return nil, err
	}
	due := make([]ScheduledTask, 0)
	for _, task := range tasks {
		if !task.Enabled || task.Running || task.NextRunAt.IsZero() || task.NextRunAt.After(now) {
			continue
		}
		due = append(due, task)
	}
	return cloneScheduledTasks(due), nil
}

func (s *Store) PrepareScheduledTaskRun(id string, manual bool, now time.Time) (ScheduledTaskRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ScheduledTaskRun{}, fmt.Errorf("scheduled task id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return ScheduledTaskRun{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ScheduledTaskRun{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	task := tasks[index]
	if task.Running {
		return ScheduledTaskRun{}, fmt.Errorf("scheduled task is already running: %s", task.Title)
	}
	if !manual {
		if !task.Enabled {
			return ScheduledTaskRun{}, fmt.Errorf("scheduled task is disabled: %s", task.Title)
		}
		if task.NextRunAt.IsZero() || task.NextRunAt.After(now) {
			return ScheduledTaskRun{}, fmt.Errorf("scheduled task is not due: %s", task.Title)
		}
	}
	if strings.TrimSpace(task.SessionID) == "" {
		session := &Session{
			ID:        NewID(),
			Title:     "定时任务：" + task.Title,
			CreatedAt: now,
			UpdatedAt: now,
			Messages:  []Message{},
		}
		s.sessions[session.ID] = session
		if err := s.saveSessionLocked(session); err != nil {
			return ScheduledTaskRun{}, err
		}
		task.SessionID = session.ID
	}
	session, ok := s.sessions[task.SessionID]
	if !ok {
		return ScheduledTaskRun{}, ErrSessionNotFound
	}
	message := strings.TrimSpace(task.Prompt)
	session.Messages = append(session.Messages, Message{Role: "user", Content: message, CreatedAt: now})
	session.UpdatedAt = now
	if err := s.saveSessionLocked(session); err != nil {
		return ScheduledTaskRun{}, err
	}

	task.Running = true
	task.UpdatedAt = now
	tasks[index] = task
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return ScheduledTaskRun{}, err
	}

	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return ScheduledTaskRun{}, err
	}
	cfg.Skills = skills
	return ScheduledTaskRun{Task: task, PromptName: s.activePrompt, SessionID: task.SessionID, Config: cfg, History: cloneMessages(session.Messages)}, nil
}

func (s *Store) FinishScheduledTaskRun(promptName string, taskID string, sessionID string, answer string, startedAt time.Time, manual bool, runErr error) (ScheduledTaskRunResponse, error) {
	promptName = strings.TrimSpace(promptName)
	if promptName == "" {
		return ScheduledTaskRunResponse{}, fmt.Errorf("prompt name is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var result ScheduledTaskRunResponse
	err := s.withPromptLocked(promptName, func() error {
		if strings.TrimSpace(answer) != "" {
			session, ok := s.sessions[sessionID]
			if !ok {
				return ErrSessionNotFound
			}
			now := time.Now()
			session.Messages = append(session.Messages, Message{Role: "assistant", Content: strings.TrimSpace(answer), CreatedAt: now})
			session.UpdatedAt = now
			if err := s.saveSessionLocked(session); err != nil {
				return err
			}
			result.Session = cloneSession(session)
		}

		tasks, err := s.loadScheduledTasksLocked()
		if err != nil {
			return err
		}
		index := -1
		for i, task := range tasks {
			if task.ID == taskID {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("scheduled task not found: %s", taskID)
		}
		task := tasks[index]
		task.Running = false
		task.SessionID = sessionID
		task.LastRunAt = &startedAt
		task.UpdatedAt = time.Now()
		if runErr != nil {
			task.LastStatus = "failed"
			task.LastError = runErr.Error()
		} else {
			task.LastStatus = "success"
			task.LastError = ""
		}
		if !manual {
			task = advanceScheduledTask(task, startedAt)
		}
		tasks[index] = task
		if err := s.saveScheduledTasksLocked(tasks); err != nil {
			return err
		}
		result.Task = task
		return nil
	})
	if err != nil {
		return ScheduledTaskRunResponse{}, err
	}
	return result, nil
}

func (s *Store) withPromptLocked(name string, fn func() error) error {
	name, err := normalizePromptName(name)
	if err != nil {
		return err
	}
	previous := s.activePrompt
	if previous != name {
		if err := s.loadPromptLocked(name); err != nil {
			return err
		}
	}
	err = fn()
	if previous != name {
		if restoreErr := s.loadPromptLocked(previous); err == nil {
			err = restoreErr
		}
	}
	return err
}

func (s *Store) loadScheduledTasksLocked() ([]ScheduledTask, error) {
	raw, ok, err := s.getPromptRawLocked(s.activePrompt, "scheduled_tasks")
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		tasks := []ScheduledTask{}
		return tasks, s.saveScheduledTasksLocked(tasks)
	}
	var tasks []ScheduledTask
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

func (s *Store) saveScheduledTasksLocked(tasks []ScheduledTask) error {
	sortScheduledTasks(tasks)
	return s.setPromptJSONLocked(s.activePrompt, "scheduled_tasks", tasks)
}

func sortScheduledTasks(tasks []ScheduledTask) {
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

func normalizeScheduledTaskInput(input ScheduledTaskRequest, previous *ScheduledTask, now time.Time) (ScheduledTask, error) {
	title, err := normalizeScheduledTaskTitle(input.Title)
	if err != nil {
		return ScheduledTask{}, err
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return ScheduledTask{}, fmt.Errorf("scheduled task prompt is empty")
	}
	if len([]byte(prompt)) > 40000 {
		return ScheduledTask{}, fmt.Errorf("scheduled task prompt is too large")
	}
	scheduleType := strings.ToLower(strings.TrimSpace(input.ScheduleType))
	if scheduleType == "" {
		scheduleType = scheduleTypeOnce
	}
	task := ScheduledTask{Title: title, Prompt: prompt, Enabled: input.Enabled, ScheduleType: scheduleType}
	switch scheduleType {
	case scheduleTypeOnce:
		runAt, err := parseTaskTime(input.RunAt)
		if err != nil {
			return ScheduledTask{}, err
		}
		task.RunAt = &runAt
		task.NextRunAt = runAt
	case scheduleTypeDaily:
		tod, err := normalizeTimeOfDay(input.TimeOfDay)
		if err != nil {
			return ScheduledTask{}, err
		}
		task.TimeOfDay = tod
		task.NextRunAt = nextDailyRun(now, tod)
	case scheduleTypeInterval:
		if input.IntervalMinutes <= 0 {
			return ScheduledTask{}, fmt.Errorf("interval_minutes must be greater than 0")
		}
		if input.IntervalMinutes > 525600 {
			return ScheduledTask{}, fmt.Errorf("interval_minutes is too large")
		}
		task.IntervalMinutes = input.IntervalMinutes
		task.NextRunAt = now.Add(time.Duration(input.IntervalMinutes) * time.Minute)
	default:
		return ScheduledTask{}, fmt.Errorf("unsupported schedule_type: %s", scheduleType)
	}
	if previous != nil && sameScheduledTaskPlan(task, *previous) && !previous.NextRunAt.IsZero() {
		task.NextRunAt = previous.NextRunAt
	}
	return task, nil
}

func sameScheduledTaskPlan(next ScheduledTask, previous ScheduledTask) bool {
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

func advanceScheduledTask(task ScheduledTask, now time.Time) ScheduledTask {
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

func cloneScheduledTasks(tasks []ScheduledTask) []ScheduledTask {
	out := make([]ScheduledTask, len(tasks))
	copy(out, tasks)
	return out
}

type DueScheduledTask struct {
	PromptName string
	Task       ScheduledTask
}

func (s *Store) DueScheduledTasksAllPrompts(now time.Time) (items []DueScheduledTask, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.activePrompt
	// 扫描所有工作空间只是后台任务的内部动作，不能改变用户当前正在看的工作空间。
	// 即使中途遇到 SQLite / 外置盘 I/O 错误，也必须尽量恢复，否则前端会突然只看到
	// 某个残留工作空间的会话，看起来像“聊天记录丢了”。
	defer func() {
		if previous == "" || s.activePrompt == previous {
			return
		}
		if restoreErr := s.loadPromptLocked(previous); restoreErr != nil && err == nil {
			err = restoreErr
		}
	}()

	prompts, err := s.listPromptNamesLocked()
	if err != nil {
		return nil, err
	}
	out := make([]DueScheduledTask, 0)
	for _, prompt := range prompts {
		if err := s.loadPromptLocked(prompt); err != nil {
			return nil, err
		}
		tasks, err := s.loadScheduledTasksLocked()
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if !task.Enabled || task.Running || task.NextRunAt.IsZero() || task.NextRunAt.After(now) {
				continue
			}
			out = append(out, DueScheduledTask{PromptName: prompt, Task: task})
		}
	}
	return out, nil
}

func (s *Store) PrepareScheduledTaskRunInPrompt(promptName string, id string, manual bool, now time.Time) (ScheduledTaskRun, error) {
	promptName, err := normalizePromptName(promptName)
	if err != nil {
		return ScheduledTaskRun{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var run ScheduledTaskRun
	err = s.withPromptLocked(promptName, func() error {
		prepared, err := s.prepareScheduledTaskRunLocked(id, manual, now)
		if err != nil {
			return err
		}
		run = prepared
		return nil
	})
	if err != nil {
		return ScheduledTaskRun{}, err
	}
	return run, nil
}

func (s *Store) prepareScheduledTaskRunLocked(id string, manual bool, now time.Time) (ScheduledTaskRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ScheduledTaskRun{}, fmt.Errorf("scheduled task id is empty")
	}
	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return ScheduledTaskRun{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ScheduledTaskRun{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	task := tasks[index]
	if task.Running {
		return ScheduledTaskRun{}, fmt.Errorf("scheduled task is already running: %s", task.Title)
	}
	if !manual {
		if !task.Enabled {
			return ScheduledTaskRun{}, fmt.Errorf("scheduled task is disabled: %s", task.Title)
		}
		if task.NextRunAt.IsZero() || task.NextRunAt.After(now) {
			return ScheduledTaskRun{}, fmt.Errorf("scheduled task is not due: %s", task.Title)
		}
	}
	if strings.TrimSpace(task.SessionID) == "" {
		session := &Session{ID: NewID(), Title: "定时任务：" + task.Title, CreatedAt: now, UpdatedAt: now, Messages: []Message{}}
		s.sessions[session.ID] = session
		if err := s.saveSessionLocked(session); err != nil {
			return ScheduledTaskRun{}, err
		}
		task.SessionID = session.ID
	}
	session, ok := s.sessions[task.SessionID]
	if !ok {
		return ScheduledTaskRun{}, ErrSessionNotFound
	}
	message := strings.TrimSpace(task.Prompt)
	session.Messages = append(session.Messages, Message{Role: "user", Content: message, CreatedAt: now})
	session.UpdatedAt = now
	if err := s.saveSessionLocked(session); err != nil {
		return ScheduledTaskRun{}, err
	}
	task.Running = true
	task.UpdatedAt = now
	tasks[index] = task
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return ScheduledTaskRun{}, err
	}
	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return ScheduledTaskRun{}, err
	}
	cfg.Skills = skills
	return ScheduledTaskRun{Task: task, PromptName: s.activePrompt, SessionID: task.SessionID, Config: cfg, History: cloneMessages(session.Messages)}, nil
}
