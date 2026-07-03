package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) DueScheduledTasks(now time.Time) ([]model.ScheduledTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return nil, err
	}
	due := make([]model.ScheduledTask, 0)
	for _, task := range tasks {
		if !task.Enabled || task.Running || task.NextRunAt.IsZero() || task.NextRunAt.After(now) {
			continue
		}
		due = append(due, task)
	}
	return cloneScheduledTasks(due), nil
}

func (s *Store) PrepareScheduledTaskRun(id string, manual bool, now time.Time) (model.ScheduledTaskRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScheduledTaskRun{}, fmt.Errorf("scheduled task id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return model.ScheduledTaskRun{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	task := tasks[index]
	if task.Running {
		return model.ScheduledTaskRun{}, fmt.Errorf("scheduled task is already running: %s", task.Title)
	}
	if !manual {
		if !task.Enabled {
			return model.ScheduledTaskRun{}, fmt.Errorf("scheduled task is disabled: %s", task.Title)
		}
		if task.NextRunAt.IsZero() || task.NextRunAt.After(now) {
			return model.ScheduledTaskRun{}, fmt.Errorf("scheduled task is not due: %s", task.Title)
		}
	}
	if strings.TrimSpace(task.SessionID) == "" {
		session := &model.Session{
			ID:        model.NewID(),
			Title:     "定时任务：" + task.Title,
			CreatedAt: now,
			UpdatedAt: now,
			Messages:  []model.Message{},
		}
		s.sessions[session.ID] = session
		if err := s.saveSessionLocked(session); err != nil {
			return model.ScheduledTaskRun{}, err
		}
		task.SessionID = session.ID
	}
	session, ok := s.sessions[task.SessionID]
	if !ok {
		return model.ScheduledTaskRun{}, model.ErrSessionNotFound
	}
	message := strings.TrimSpace(task.Prompt)
	session.Messages = append(session.Messages, model.Message{Role: "user", Content: message, CreatedAt: now})
	session.UpdatedAt = now
	if err := s.saveSessionLocked(session); err != nil {
		return model.ScheduledTaskRun{}, err
	}

	task.Running = true
	task.UpdatedAt = now
	tasks[index] = task
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return model.ScheduledTaskRun{}, err
	}

	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	cfg.Skills = skills
	return model.ScheduledTaskRun{Task: task, PromptName: s.activePrompt, SessionID: task.SessionID, Config: cfg, History: cloneMessages(session.Messages)}, nil
}

func (s *Store) FinishScheduledTaskRun(promptName string, taskID string, sessionID string, answer string, startedAt time.Time, manual bool, runErr error) (model.ScheduledTaskRunResponse, error) {
	promptName = strings.TrimSpace(promptName)
	if promptName == "" {
		return model.ScheduledTaskRunResponse{}, fmt.Errorf("prompt name is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var result model.ScheduledTaskRunResponse
	err := s.withPromptLocked(promptName, func() error {
		if strings.TrimSpace(answer) != "" {
			session, ok := s.sessions[sessionID]
			if !ok {
				return model.ErrSessionNotFound
			}
			now := time.Now()
			session.Messages = append(session.Messages, model.Message{Role: "assistant", Content: strings.TrimSpace(answer), CreatedAt: now})
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
		return model.ScheduledTaskRunResponse{}, err
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
