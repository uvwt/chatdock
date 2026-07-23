package store

import "strings"

// scheduledSessionIDsLocked 返回由定时任务持有或历史运行引用的会话。
// 这些会话仍可从定时任务运行记录进入，但不属于普通聊天会话列表和搜索结果。
func (s *Store) scheduledSessionIDsLocked() (map[string]struct{}, error) {
	rows, err := s.db.Query(`
SELECT session_id FROM scheduled_tasks WHERE session_id != ''
UNION
SELECT session_id FROM scheduled_task_runs WHERE session_id != ''
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
