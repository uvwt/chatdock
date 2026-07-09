package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type DueScheduledTask struct {
	WorkspaceID string
	Task        model.ScheduledTask
}

func (s *Store) DueScheduledTasksAllWorkspaces(now time.Time) (items []DueScheduledTask, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT workspace_id, id, title, task_prompt, enabled, running, schedule_type, run_at, time_of_day, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at FROM scheduled_tasks WHERE enabled = 1 AND running = 0 AND next_run_at != '' AND next_run_at <= ? ORDER BY next_run_at ASC`, formatScheduleDBTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DueScheduledTask, 0)
	for rows.Next() {
		var workspaceID string
		var task model.ScheduledTask
		var enabled, running int
		var runAt, nextRunAt, lastRunAt, createdAt, updatedAt string
		if err := rows.Scan(&workspaceID, &task.ID, &task.Title, &task.Prompt, &enabled, &running, &task.ScheduleType, &runAt, &task.TimeOfDay, &task.IntervalMinutes, &task.ContextMode, &nextRunAt, &lastRunAt, &task.LastStatus, &task.LastError, &task.SessionID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		task.Enabled = enabled != 0
		task.Running = running != 0
		task.RunAt = parseOptionalDBTime(runAt)
		task.NextRunAt = parseDBTimeZero(nextRunAt)
		task.LastRunAt = parseOptionalDBTime(lastRunAt)
		task.CreatedAt = parseDBTimeZero(createdAt)
		task.UpdatedAt = parseDBTimeZero(updatedAt)
		task.ContextMode = normalizeScheduledTaskContextMode(task.ContextMode)
		out = append(out, DueScheduledTask{WorkspaceID: workspaceID, Task: task})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) PrepareScheduledTaskRunInWorkspace(workspaceID string, id string, manual bool, now time.Time) (model.ScheduledTaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	return s.prepareScheduledTaskRunLocked(workspaceID, id, manual, now)
}

func (s *Store) prepareScheduledTaskRunLocked(workspaceID string, id string, manual bool, now time.Time) (model.ScheduledTaskRun, error) {
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScheduledTaskRun{}, fmt.Errorf("scheduled task id is empty")
	}
	tasks, err := s.loadScheduledTasksForWorkspaceLocked(workspaceID)
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
	task.ContextMode = normalizeScheduledTaskContextMode(task.ContextMode)
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
	runID := model.NewID()
	message := strings.TrimSpace(task.Prompt)
	userMessage := model.Message{ID: model.NewID(), Role: "user", Content: message, CreatedAt: now}
	history := []model.Message{userMessage}
	sessionID := ""
	cfg, err := s.modelConfigForWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	switch task.ContextMode {
	case model.ScheduledTaskContextSession:
		if strings.TrimSpace(task.SessionID) == "" {
			session := &model.Session{ID: model.NewID(), Title: "定时任务：" + task.Title, CreatedAt: now, UpdatedAt: now, Messages: []model.Message{}}
			if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
				return model.ScheduledTaskRun{}, err
			}
			task.SessionID = session.ID
		}
		session, ok, err := s.sessionForWorkspaceLocked(workspaceID, task.SessionID)
		if err != nil {
			return model.ScheduledTaskRun{}, err
		}
		if !ok {
			return model.ScheduledTaskRun{}, model.ErrSessionNotFound
		}
		session.Messages = append(session.Messages, userMessage)
		session.UpdatedAt = now
		if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
			return model.ScheduledTaskRun{}, err
		}
		sessionID = task.SessionID
		history = cloneMessages(session.Messages)
	case model.ScheduledTaskContextLastResult:
		session, err := s.createScheduledTaskRunSessionLocked(workspaceID, cfg, task, userMessage, now)
		if err != nil {
			return model.ScheduledTaskRun{}, err
		}
		sessionID = session.ID
		if previous, ok, err := s.latestSuccessfulScheduledTaskRunLocked(workspaceID, task.ID); err != nil {
			return model.ScheduledTaskRun{}, err
		} else if ok {
			history = []model.Message{{ID: model.NewID(), Role: "system", Content: "以下是这个定时任务上次成功运行的结果，仅用于延续状态，不代表本次已经完成：\n" + limitRunes(previous.Output, 8000), CreatedAt: now}, userMessage}
		}
	default:
		session, err := s.createScheduledTaskRunSessionLocked(workspaceID, cfg, task, userMessage, now)
		if err != nil {
			return model.ScheduledTaskRun{}, err
		}
		sessionID = session.ID
	}
	task.Running = true
	task.UpdatedAt = now
	tasks[index] = task
	if err := s.saveScheduledTasksForWorkspaceLocked(workspaceID, tasks); err != nil {
		return model.ScheduledTaskRun{}, err
	}
	return model.ScheduledTaskRun{Task: task, WorkspaceID: workspaceID, SessionID: sessionID, RunID: runID, Config: cfg, History: history}, nil
}

func (s *Store) createScheduledTaskRunSessionLocked(workspaceID string, cfg model.ModelConfig, task model.ScheduledTask, userMessage model.Message, now time.Time) (*model.Session, error) {
	title := "定时任务：" + task.Title
	if task.ContextMode != model.ScheduledTaskContextSession {
		title = title + " · " + now.Format("01-02 15:04")
	}
	session := &model.Session{ID: model.NewID(), Title: title, ProviderID: cfg.ProviderID, Model: cfg.Model, CreatedAt: now, UpdatedAt: now, Messages: []model.Message{userMessage}}
	if err := s.saveSessionForWorkspaceLocked(workspaceID, session); err != nil {
		return nil, err
	}
	return session, nil
}
