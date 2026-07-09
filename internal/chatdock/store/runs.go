package store

import (
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
)

func (s *Store) StartMCPRun(workspaceID string, sessionID string, title string) (MCPRun, error) {
	now := time.Now()
	title = strings.TrimSpace(title)
	if title == "" {
		title = "MCP tool run"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return MCPRun{}, err
	}
	run := MCPRun{ID: model.NewID(), Workspace: workspaceID, SessionID: strings.TrimSpace(sessionID), Title: title, Status: "running", StartedAt: now, UpdatedAt: now}
	_, err = s.db.Exec(`INSERT INTO mcp_runs(workspace_id, id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.Workspace, run.ID, run.SessionID, run.Title, run.Status, run.Summary, run.Error, formatDBTime(run.StartedAt), "", run.DurationMS, run.EventCount, formatDBTime(run.UpdatedAt))
	return run, err
}

func (s *Store) AddMCPRunEvent(runID string, input RunEventInput) (MCPRunEvent, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return MCPRunEvent{}, fmt.Errorf("run id is empty")
	}
	now := time.Now()
	started := input.StartedAt
	if started.IsZero() {
		started = now
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "success"
	}
	server, tool := mcp.SplitToolFullName(strings.TrimSpace(input.Tool))
	if strings.TrimSpace(input.Tool) == "" {
		server, tool = "", ""
	}
	action := actionFromArguments(input.Arguments)
	event := MCPRunEvent{ID: model.NewID(), RunID: runID, Kind: llm.FirstNonEmptyString(input.Kind, "tool_call"), Status: status, Server: server, Tool: tool, Action: action, Summary: summarizeRunEvent(status, input.Tool, action, input.Error), Arguments: input.Arguments, Result: input.Result, Error: strings.TrimSpace(input.Error), DurationMS: input.DurationMS, StartedAt: started, FinishedAt: input.FinishedAt, CreatedAt: now}
	if event.FinishedAt == nil && status != "running" {
		finished := now
		event.FinishedAt = &finished
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var seq int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM mcp_run_events WHERE run_id = ?`, runID).Scan(&seq); err != nil {
		return MCPRunEvent{}, err
	}
	event.Seq = seq
	finishedRaw := ""
	if event.FinishedAt != nil {
		finishedRaw = formatDBTime(*event.FinishedAt)
	}
	_, err := s.db.Exec(`INSERT INTO mcp_run_events(id, run_id, seq, kind, status, server, tool, action, summary, arguments_json, result_json, error, duration_ms, started_at, finished_at, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.RunID, event.Seq, event.Kind, event.Status, event.Server, event.Tool, event.Action, event.Summary, compactJSONForDB(event.Arguments), compactJSONForDB(event.Result), event.Error, event.DurationMS, formatDBTime(event.StartedAt), finishedRaw, formatDBTime(event.CreatedAt))
	if err != nil {
		return MCPRunEvent{}, err
	}
	_, err = s.db.Exec(`UPDATE mcp_runs SET event_count = event_count + 1, summary = ?, updated_at = ? WHERE id = ?`, event.Summary, formatDBTime(now), runID)
	return event, err
}

func (s *Store) FinishMCPRun(runID string, status string, summary string, runErr error) (MCPRun, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "success"
	}
	errorText := ""
	if runErr != nil {
		status = "failed"
		errorText = runErr.Error()
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var startedRaw string
	if err := s.db.QueryRow(`SELECT started_at FROM mcp_runs WHERE id = ?`, runID).Scan(&startedRaw); err != nil {
		return MCPRun{}, err
	}
	started := parseDBTime(startedRaw)
	durationMS := now.Sub(started).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	_, err := s.db.Exec(`UPDATE mcp_runs SET status = ?, summary = ?, error = ?, finished_at = ?, duration_ms = ?, updated_at = ? WHERE id = ?`, status, strings.TrimSpace(summary), errorText, formatDBTime(now), durationMS, formatDBTime(now), runID)
	if err != nil {
		return MCPRun{}, err
	}
	return s.getMCPRunLocked(runID)
}

func (s *Store) ListMCPRuns(workspaceID string, sessionID string, limit int) (MCPRunResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	sessionID = strings.TrimSpace(sessionID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	workspaceID, err := s.requireWorkspaceLocked(workspaceID)
	if err != nil {
		return MCPRunResponse{}, err
	}
	query := `SELECT workspace_id, id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at FROM mcp_runs WHERE workspace_id = ?`
	args := []any{workspaceID}
	if sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return MCPRunResponse{}, err
	}
	defer rows.Close()
	runs, err := scanMCPRuns(rows)
	if err != nil {
		return MCPRunResponse{}, err
	}
	return MCPRunResponse{Runs: runs}, nil
}

func (s *Store) MCPRunDetail(workspaceID string, runID string) (MCPRunDetailResponse, error) {
	workspaceID, err := normalizeWorkspaceID(workspaceID)
	if err != nil {
		return MCPRunDetailResponse{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, err := s.getMCPRunForWorkspaceLocked(workspaceID, runID)
	if err != nil {
		return MCPRunDetailResponse{}, err
	}
	events, err := s.listMCPRunEventsLocked(runID)
	if err != nil {
		return MCPRunDetailResponse{}, err
	}
	return MCPRunDetailResponse{Run: run, Events: events}, nil
}
