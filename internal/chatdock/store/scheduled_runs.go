package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

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
	session, sessionChanged, err := s.prepareScheduledTaskSessionCompletionLocked(workspaceID, sessionID, answer, runErr, assistantAlreadySaved, finishedAt)
	if err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	task, err := s.scheduledTaskByIDLocked(workspaceID, taskID)
	if err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	task, status, errorText := finishScheduledTaskState(task, sessionID, startedAt, finishedAt, manual, runErr)
	record := normalizeScheduledRunRecordForDB(model.ScheduledTaskRunRecord{
		ID: runID, TaskID: task.ID, TaskTitle: task.Title, Prompt: task.Prompt, Output: answer,
		Status: status, Error: errorText, Manual: manual, SessionID: sessionID,
		StartedAt: startedAt, FinishedAt: &finishedAt,
	})
	if err := s.saveScheduledTaskCompletionLocked(workspaceID, task, record, session, sessionChanged); err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	return model.ScheduledTaskRunResponse{Task: task, Run: &record, Session: cloneSession(session)}, nil
}

func (s *Store) prepareScheduledTaskSessionCompletionLocked(workspaceID string, sessionID string, answer string, runErr error, assistantAlreadySaved bool, finishedAt time.Time) (*model.Session, bool, error) {
	if sessionID == "" {
		return nil, false, nil
	}
	session, ok, err := s.sessionForWorkspaceLocked(workspaceID, sessionID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, model.ErrSessionNotFound
	}
	if assistantAlreadySaved {
		return session, false, nil
	}
	assistantContent := answer
	if runErr != nil {
		assistantContent = "运行失败：" + strings.TrimSpace(runErr.Error())
	} else if assistantContent == "" {
		assistantContent = "模型没有返回内容。"
	}
	session.Messages = append(session.Messages, model.Message{ID: model.NewID(), Role: "assistant", Content: assistantContent, CreatedAt: finishedAt})
	session.UpdatedAt = finishedAt
	return session, true, nil
}

func (s *Store) scheduledTaskByIDLocked(workspaceID string, taskID string) (model.ScheduledTask, error) {
	tasks, err := s.loadScheduledTasksForWorkspaceLocked(workspaceID)
	if err != nil {
		return model.ScheduledTask{}, err
	}
	for _, task := range tasks {
		if task.ID == taskID {
			return task, nil
		}
	}
	return model.ScheduledTask{}, fmt.Errorf("scheduled task not found: %s", taskID)
}

func finishScheduledTaskState(task model.ScheduledTask, sessionID string, startedAt time.Time, finishedAt time.Time, manual bool, runErr error) (model.ScheduledTask, string, string) {
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
	}
	task.LastStatus = status
	task.LastError = errorText
	if !manual {
		task = advanceScheduledTask(task, startedAt)
	}
	return task, status, errorText
}

func (s *Store) saveScheduledTaskCompletionLocked(workspaceID string, task model.ScheduledTask, record model.ScheduledTaskRunRecord, session *model.Session, sessionChanged bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if sessionChanged {
		if err := upsertSessionTablesTx(tx, workspaceID, session); err != nil {
			return err
		}
	}
	if err := upsertScheduledTaskTx(tx, workspaceID, normalizeScheduledTaskForDB(task)); err != nil {
		return err
	}
	if err := upsertScheduledTaskRunTx(tx, workspaceID, record); err != nil {
		return err
	}
	if err := touchWorkspace(tx, workspaceID, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}
