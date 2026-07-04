package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type DueScheduledTask struct {
	PromptName string
	Task       model.ScheduledTask
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

func (s *Store) PrepareScheduledTaskRunInPrompt(promptName string, id string, manual bool, now time.Time) (model.ScheduledTaskRun, error) {
	promptName, err := normalizePromptName(promptName)
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var run model.ScheduledTaskRun
	err = s.withPromptLocked(promptName, func() error {
		prepared, err := s.prepareScheduledTaskRunLocked(id, manual, now)
		if err != nil {
			return err
		}
		run = prepared
		return nil
	})
	if err != nil {
		return model.ScheduledTaskRun{}, err
	}
	return run, nil
}

func (s *Store) prepareScheduledTaskRunLocked(id string, manual bool, now time.Time) (model.ScheduledTaskRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScheduledTaskRun{}, fmt.Errorf("scheduled task id is empty")
	}
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
	history := []model.Message{{ID: model.NewID(), Role: "user", Content: message, CreatedAt: now}}
	sessionID := ""

	switch task.ContextMode {
	case model.ScheduledTaskContextSession:
		if strings.TrimSpace(task.SessionID) == "" {
			session := &model.Session{ID: model.NewID(), Title: "定时任务：" + task.Title, CreatedAt: now, UpdatedAt: now, Messages: []model.Message{}}
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
		session.Messages = append(session.Messages, model.Message{ID: model.NewID(), Role: "user", Content: message, CreatedAt: now})
		session.UpdatedAt = now
		if err := s.saveSessionLocked(session); err != nil {
			return model.ScheduledTaskRun{}, err
		}
		sessionID = task.SessionID
		history = cloneMessages(session.Messages)
	case model.ScheduledTaskContextLastResult:
		if previous, ok, err := s.latestSuccessfulScheduledTaskRunLocked(task.ID); err != nil {
			return model.ScheduledTaskRun{}, err
		} else if ok {
			history = []model.Message{
				{ID: model.NewID(), Role: "system", Content: "以下是这个定时任务上次成功运行的结果，仅用于延续状态，不代表本次已经完成：\n" + limitRunes(previous.Output, 8000), CreatedAt: now},
				{ID: model.NewID(), Role: "user", Content: message, CreatedAt: now},
			}
		}
	default:
		// stateless: 只发送本次任务提示词，不带历史会话，避免定时任务长期积累 token。
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
	return model.ScheduledTaskRun{Task: task, PromptName: s.activePrompt, SessionID: sessionID, RunID: runID, Config: cfg, History: history}, nil
}
