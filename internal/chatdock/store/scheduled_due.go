package store

import (
	"chatdock/internal/chatdock/schedule"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type DueScheduledTask struct {
	Task model.ScheduledTask
}

func (s *Store) DueScheduledTasks(now time.Time) (items []DueScheduledTask, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id, title, task_prompt, enabled, running, schedule_type, run_at, cron_expressions, timezone, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at FROM scheduled_tasks WHERE enabled = 1 AND running = 0 AND next_run_at != '' AND next_run_at <= ? ORDER BY next_run_at ASC`, formatScheduleDBTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DueScheduledTask, 0)
	for rows.Next() {
		var task model.ScheduledTask
		var enabled, running int
		var runAt, cronExpressions, nextRunAt, lastRunAt, createdAt, updatedAt string
		if err := rows.Scan(&task.ID, &task.Title, &task.Prompt, &enabled, &running, &task.ScheduleType, &runAt, &cronExpressions, &task.Timezone, &task.IntervalMinutes, &task.ContextMode, &nextRunAt, &lastRunAt, &task.LastStatus, &task.LastError, &task.SessionID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cronExpressions), &task.CronExpressions); err != nil {
			return nil, fmt.Errorf("decode scheduled task %s cron expressions: %w", task.ID, err)
		}
		task.Enabled = enabled != 0
		task.Running = running != 0
		task.RunAt = parseOptionalDBTime(runAt)
		task.NextRunAt = parseDBTimeZero(nextRunAt)
		task.LastRunAt = parseOptionalDBTime(lastRunAt)
		task.CreatedAt = parseDBTimeZero(createdAt)
		task.UpdatedAt = parseDBTimeZero(updatedAt)
		task.ContextMode = schedule.NormalizeContextMode(task.ContextMode)
		out = append(out, DueScheduledTask{Task: task})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) PrepareScheduledTaskRun(id string, manual bool, now time.Time) (model.ScheduledTaskRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepareScheduledTaskRunLocked(id, manual, now)
}

func (s *Store) prepareScheduledTaskRunLocked(id string, manual bool, now time.Time) (model.ScheduledTaskRun, error) {
	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	_, task, err := scheduledTaskForRun(tasks, id, manual, now)
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	cfg, err := s.modelConfigLocked()
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	userMessage := model.Message{ID: model.NewID(), Role: "user", Content: strings.TrimSpace(task.Prompt), CreatedAt: now}
	task, session, history, err := s.prepareScheduledTaskContextLocked(cfg, task, userMessage, now)
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	// 所有定时任务模式都立即记录当前会话，确保运行中的会话也不会混入普通会话列表。
	task.SessionID = session.ID
	task.Running = true
	task.UpdatedAt = now
	if err := s.saveScheduledTaskStartLocked(task, session); err != nil {
		return model.ScheduledTaskRun{}, err
	}
	return model.ScheduledTaskRun{Task: task, SessionID: session.ID, RunID: model.NewID(), Config: cfg, History: history}, nil
}

func scheduledTaskForRun(tasks []model.ScheduledTask, id string, manual bool, now time.Time) (int, model.ScheduledTask, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1, model.ScheduledTask{}, fmt.Errorf("scheduled task id is empty")
	}
	for index, task := range tasks {
		if task.ID != id {
			continue
		}
		task.ContextMode = schedule.NormalizeContextMode(task.ContextMode)
		if task.Running {
			return -1, model.ScheduledTask{}, fmt.Errorf("scheduled task is already running: %s", task.Title)
		}
		if !manual && !task.Enabled {
			return -1, model.ScheduledTask{}, fmt.Errorf("scheduled task is disabled: %s", task.Title)
		}
		if !manual && (task.NextRunAt.IsZero() || task.NextRunAt.After(now)) {
			return -1, model.ScheduledTask{}, fmt.Errorf("scheduled task is not due: %s", task.Title)
		}
		return index, task, nil
	}
	return -1, model.ScheduledTask{}, fmt.Errorf("scheduled task not found: %s", id)
}

func (s *Store) prepareScheduledTaskContextLocked(cfg model.ModelConfig, task model.ScheduledTask, userMessage model.Message, now time.Time) (model.ScheduledTask, *model.Session, []model.Message, error) {
	history := []model.Message{userMessage}
	switch task.ContextMode {
	case model.ScheduledTaskContextSession:
		session, updatedTask, err := s.prepareScheduledTaskSessionLocked(task, userMessage, now)
		if err != nil {
			return task, nil, nil, err
		}
		return updatedTask, session, cloneMessages(session.Messages), nil
	case model.ScheduledTaskContextLastResult:
		session := newScheduledTaskRunSession(cfg, userMessage, now)
		previous, ok, err := s.latestSuccessfulScheduledTaskRunLocked(task.ID)
		if err != nil {
			return task, nil, nil, err
		}
		if ok {
			history = []model.Message{{ID: model.NewID(), Role: "system", Content: "以下是这个定时任务上次成功运行的结果，仅用于延续状态，不代表本次已经完成：\n" + limitRunes(previous.Output, 8000), CreatedAt: now}, userMessage}
		}
		return task, session, history, nil
	default:
		return task, newScheduledTaskRunSession(cfg, userMessage, now), history, nil
	}
}

func (s *Store) prepareScheduledTaskSessionLocked(task model.ScheduledTask, userMessage model.Message, now time.Time) (*model.Session, model.ScheduledTask, error) {
	var session *model.Session
	if strings.TrimSpace(task.SessionID) == "" {
		session = &model.Session{ID: model.NewID(), Title: makeTitle(userMessage.Content), CreatedAt: now, UpdatedAt: now, Messages: []model.Message{}}
		task.SessionID = session.ID
	} else {
		var ok bool
		var err error
		session, ok, err = s.sessionLocked(task.SessionID)
		if err != nil {
			return nil, task, err
		}
		if !ok {
			return nil, task, model.ErrSessionNotFound
		}
	}
	session.Messages = append(session.Messages, userMessage)
	session.UpdatedAt = now
	return session, task, nil
}

func newScheduledTaskRunSession(cfg model.ModelConfig, userMessage model.Message, now time.Time) *model.Session {
	return &model.Session{ID: model.NewID(), Title: makeTitle(userMessage.Content), ProviderID: cfg.ProviderID, Model: cfg.Model, CreatedAt: now, UpdatedAt: now, Messages: []model.Message{userMessage}}
}

func (s *Store) saveScheduledTaskStartLocked(task model.ScheduledTask, session *model.Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertSessionTablesTx(tx, session); err != nil {
		return err
	}
	if err := upsertScheduledTaskTx(tx, normalizeScheduledTaskForDB(task)); err != nil {
		return err
	}
	return tx.Commit()
}
