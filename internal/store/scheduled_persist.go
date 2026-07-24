package store

import (
	"time"

	"chatdock/internal/model"
	"chatdock/internal/schedule"
)

func (s *Store) loadScheduledTasksLocked() ([]model.ScheduledTask, error) {
	tasks, err := loadScheduledTasksLocked(s.db)
	if err != nil {
		return nil, err
	}
	changed := false
	for i := range tasks {
		tasks[i].ContextMode = schedule.NormalizeContextMode(tasks[i].ContextMode)
		if tasks[i].Running && time.Since(tasks[i].UpdatedAt) > 2*time.Hour {
			tasks[i].Running = false
			tasks[i].LastError = "上次运行异常中断，已自动恢复为可运行状态"
			changed = true
		}
	}
	schedule.SortTasks(tasks)
	if changed {
		if err := s.saveScheduledTasksLocked(tasks); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (s *Store) saveScheduledTasksLocked(tasks []model.ScheduledTask) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	schedule.SortTasks(tasks)
	keep := map[string]bool{}
	for _, task := range tasks {
		task = normalizeScheduledTaskForDB(task)
		keep[task.ID] = true
		if err := upsertScheduledTaskTx(tx, task); err != nil {
			return err
		}
	}
	if err := deleteScheduledTasksExceptLocked(tx, keep); err != nil {
		return err
	}
	return tx.Commit()
}
