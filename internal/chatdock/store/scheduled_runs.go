package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) DueScheduledTasks(workspaceID string, now time.Time) ([]model.ScheduledTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.loadScheduledTasksForWorkspaceLocked(workspaceID)
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

func (s *Store) FinishScheduledTaskRun(workspaceID string, taskID string, runID string, sessionID string, answer string, startedAt time.Time, manual bool, runErr error, assistantAlreadySaved bool) (model.ScheduledTaskRunResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	finishedAt := time.Now()
	answer = strings.TrimSpace(answer)
	sessionID = strings.TrimSpace(sessionID)
	var result model.ScheduledTaskRunResponse
	if sessionID != "" {
		session, ok, err := s.sessionForWorkspaceLocked(workspaceID, sessionID)
		if err != nil {
			return model.ScheduledTaskRunResponse{}, err
		}
		if !ok {
			return model.ScheduledTaskRunResponse{}, model.ErrSessionNotFound
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
			if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
				return model.ScheduledTaskRunResponse{}, err
			}
		}
		result.Session = cloneSession(session)
	}
	tasks, err := s.loadScheduledTasksForWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == taskID {
			index = i
			break
		}
	}
	if index < 0 {
		return model.ScheduledTaskRunResponse{}, fmt.Errorf("scheduled task not found: %s", taskID)
	}
	task := tasks[index]
	task.ContextMode = normalizeScheduledTaskContextMode(task.ContextMode)
	task.Running = false
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
	record, err := s.appendScheduledTaskRunRecordLocked(workspaceID, model.ScheduledTaskRunRecord{ID: runID, TaskID: task.ID, TaskTitle: task.Title, Prompt: task.Prompt, Output: answer, Status: status, Error: errorText, Manual: manual, SessionID: sessionID, StartedAt: startedAt, FinishedAt: &finishedAt})
	if err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	tasks[index] = task
	if err := s.saveScheduledTasksForWorkspaceLocked(workspaceID, tasks); err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	result.Task = task
	result.Run = &record
	return result, nil
}
