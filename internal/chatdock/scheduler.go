package chatdock

import (
	"context"
	"log"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (a *App) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runDueScheduledTasks()
		}
	}
}

func (a *App) runDueScheduledTasks() {
	tasks, err := a.store.DueScheduledTasksAllWorkspaces(time.Now())
	if err != nil {
		log.Printf("scheduled task scan failed: %v", err)
		return
	}
	for _, task := range tasks {
		a.startScheduledTask(task.WorkspaceID, task.Task.ID, false)
	}
}

func (a *App) startScheduledTask(workspaceID string, id string, manual bool) {
	key := strings.TrimSpace(workspaceID) + ":" + strings.TrimSpace(id)
	if key == "" {
		return
	}
	a.runningMu.Lock()
	if a.running[key] {
		a.runningMu.Unlock()
		return
	}
	a.running[key] = true
	a.runningMu.Unlock()

	started := a.startBackgroundWork(func(ctx context.Context) {
		defer func() {
			a.runningMu.Lock()
			delete(a.running, key)
			a.runningMu.Unlock()
		}()
		if _, err := a.executeScheduledTask(ctx, workspaceID, id, manual); err != nil && !isClientCanceled(ctx, err) {
			log.Printf("scheduled task %s failed: %v", key, err)
		}
	})
	if !started {
		a.runningMu.Lock()
		delete(a.running, key)
		a.runningMu.Unlock()
	}
}

func (a *App) executeScheduledTask(ctx context.Context, workspaceID string, id string, manual bool) (model.ScheduledTaskRunResponse, error) {
	startedAt := time.Now()
	run, err := a.store.PrepareScheduledTaskRunInWorkspace(workspaceID, id, manual, startedAt)
	if err != nil {
		return model.ScheduledTaskRunResponse{}, err
	}
	completion, runErr := a.completeScheduledSessionWithRecordedEvents(ctx, workspaceID, run.SessionID, run.Config, run.History)
	result, finishErr := a.store.FinishScheduledTaskRun(run.WorkspaceID, run.Task.ID, run.RunID, run.SessionID, completion.Answer, startedAt, manual, runErr, completion.AssistantSaved)
	if finishErr != nil {
		return model.ScheduledTaskRunResponse{}, finishErr
	}
	if runErr != nil {
		return result, runErr
	}
	// 定时任务正文完成后，标题生成不再依赖触发请求；手动运行页面断开时也应完成改名。
	titleCtx := withRequestID(a.lifecycleCtx, requestIDFromContext(ctx))
	if renamed, titleErr := a.maybeGenerateSessionTitle(titleCtx, workspaceID, run.SessionID, run.Config); titleErr != nil {
		logError("session_title_generation_failed", titleErr, logFields{"session_id": run.SessionID})
	} else {
		result.Session = renamed
	}
	return result, nil
}
