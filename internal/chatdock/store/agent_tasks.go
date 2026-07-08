package store

import (
	"sort"

	"chatdock/internal/chatdock/llm"
)

func (s *Store) ListAgentTasks(limit int) (AgentTaskResponse, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	prompt := s.WorkspaceCacheID()
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT r.workspace_id, r.session_id, e.run_id, e.server, e.tool, e.action, e.status, e.summary, e.arguments_json, e.result_json, e.error, e.created_at
FROM mcp_run_events e JOIN mcp_runs r ON r.id = e.run_id
WHERE r.workspace_id = ? AND e.tool = 'task_manage'
ORDER BY e.created_at DESC LIMIT ?`, prompt, limit*4)
	if err != nil {
		return AgentTaskResponse{}, err
	}
	defer rows.Close()
	tasksByID := map[string]AgentTask{}
	for rows.Next() {
		var workspace, sessionID, runID, server, tool, action, status, summary, argsRaw, resultRaw, errorText, createdRaw string
		if err := rows.Scan(&workspace, &sessionID, &runID, &server, &tool, &action, &status, &summary, &argsRaw, &resultRaw, &errorText, &createdRaw); err != nil {
			return AgentTaskResponse{}, err
		}
		_ = tool
		args := decodeJSONMap(argsRaw)
		result := decodeJSONMap(resultRaw)
		id := llm.FirstNonEmptyString(stringFromMap(args, "task_id"), stringFromNestedResult(result, "task_id"), runID)
		if _, exists := tasksByID[id]; exists {
			continue
		}
		title := llm.FirstNonEmptyString(stringFromMap(args, "title"), stringFromMap(args, "goal"), stringFromNestedResult(result, "title"), "AgentDock task")
		tasksByID[id] = AgentTask{ID: id, Title: title, Status: agentTaskStatus(action, status, errorText, result), Workspace: workspace, SessionID: sessionID, SourceRun: runID, Server: server, Action: action, Phase: llm.FirstNonEmptyString(stringFromNestedResult(result, "phase"), stringFromNestedResult(result, "current_phase")), Summary: llm.FirstNonEmptyString(summary, stringFromNestedResult(result, "summary")), Error: errorText, UpdatedAt: parseDBTime(createdRaw)}
	}
	if err := rows.Err(); err != nil {
		return AgentTaskResponse{}, err
	}
	tasks := make([]AgentTask, 0, len(tasksByID))
	for _, task := range tasksByID {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt) })
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return AgentTaskResponse{Tasks: tasks}, nil
}
