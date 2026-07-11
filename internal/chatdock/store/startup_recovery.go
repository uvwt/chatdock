package store

import "time"

const interruptedByRestartMessage = "ChatDock restarted before the operation completed."

// recoverInterruptedWork 把上一次进程遗留的执行态工作收敛为终态。
// ChatDock 不支持多个实例共享同一数据目录，因此数据库中仍标记为 running
// 的记录只能来自异常退出；必须在新调度器和请求处理启动前一次性恢复。
// 待确认记录是可持久化的用户决策，不属于进程内执行态，重启后仍应允许处理。
func (s *Store) recoverInterruptedWork() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := formatDBTime(time.Now())

	if _, err := tx.Exec(`UPDATE chat_jobs
SET status = 'interrupted',
    error = CASE WHEN trim(error) = '' THEN ? ELSE error END,
    finished_at = CASE WHEN trim(finished_at) = '' THEN ? ELSE finished_at END,
    updated_at = ?
WHERE status = 'running'`, interruptedByRestartMessage, now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE scheduled_tasks
SET running = 0,
    last_status = 'error',
    last_error = CASE WHEN trim(last_error) = '' THEN ? ELSE last_error END,
    updated_at = ?
WHERE running = 1`, interruptedByRestartMessage, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE mcp_runs
SET status = 'error',
    error = CASE WHEN trim(error) = '' THEN ? ELSE error END,
    finished_at = CASE WHEN trim(finished_at) = '' THEN ? ELSE finished_at END,
    updated_at = ?
WHERE status = 'running'`, interruptedByRestartMessage, now, now); err != nil {
		return err
	}
	return tx.Commit()
}
