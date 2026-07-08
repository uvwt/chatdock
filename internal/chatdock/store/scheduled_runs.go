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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepareScheduledTaskRunLocked(id, manual, now)
}

func (s *Store) FinishScheduledTaskRun(workspaceID string, taskID string, runID string, sessionID string, answer string, startedAt time.Time, manual bool, runErr error, assistantAlreadySaved bool) (model.ScheduledTaskRunResponse, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return model.ScheduledTaskRunResponse{}, fmt.Errorf("workspace id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var result model.ScheduledTaskRunResponse
	err := s.withWorkspaceCacheLocked(workspaceID, func() error {
		finishedAt := time.Now()
		answer = strings.TrimSpace(answer)
		sessionID = strings.TrimSpace(sessionID)
		if sessionID != "" {
			session, ok := s.sessions[sessionID]
			if !ok {
				return model.ErrSessionNotFound
			}
			if !assistantAlreadySaved {
				assistantContent := answer
				if runErr != nil {
					assistantContent = "运行失败：" + strings.TrimSpace(runErr.Error())
				} else if assistantContent == "" {
					assistantContent = "模型没有返回内容。"
				}
				session.Messages = append(session.Messages, model.Message{ID: model.NewID(), Role: "assistant", Content: assistantContent, CreatedAt: finishedAt})
				session.UpdatedAt = finishedAt
				if err := s.saveSessionLocked(session); err != nil {
					return err
				}
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
		task.ContextMode = normalizeScheduledTaskContextMode(task.ContextMode)
		task.Running = false
		// SessionID 总是记录最近一次运行会话；只有 session 模式会在下次运行时复用它。
		task.SessionID = sessionID
		task.LastRunAt = &startedAt
		task.UpdatedAt = finishedAt
		status := "success"
		errorText := ""
		if runErr != nil {
			status = "failed"
			errorText = runErr.Error()
			task.LastStatus = "failed"
			task.LastError = errorText
		} else {
			task.LastStatus = "success"
			task.LastError = ""
		}
		if !manual {
			task = advanceScheduledTask(task, startedAt)
		}
		record, err := s.appendScheduledTaskRunRecordLocked(model.ScheduledTaskRunRecord{
			ID:         runID,
			TaskID:     task.ID,
			TaskTitle:  task.Title,
			Prompt:     task.Prompt,
			Output:     answer,
			Status:     status,
			Error:      errorText,
			Manual:     manual,
			SessionID:  sessionID,
			StartedAt:  startedAt,
			FinishedAt: &finishedAt,
		})
		if err != nil {
			return err
		}
		tasks[index] = task
		if err := s.saveScheduledTasksLocked(tasks); err != nil {
			return err
		}
		result.Task = task
		result.Run = &record
		return nil
	})
	if err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	return result, nil
}

func (s *Store) withWorkspaceCacheLocked(name string, fn func() error) error {
	name, err := normalizeWorkspaceID(name)
	if err != nil {
		return err
	}
	previous := s.workspaceCacheID
	if previous != name {
		if err := s.loadWorkspaceLocked(name); err != nil {
			return err
		}
	}
	err = fn()
	if previous != name {
		if restoreErr := s.loadWorkspaceLocked(previous); err == nil {
			err = restoreErr
		}
	}
	return err
}
