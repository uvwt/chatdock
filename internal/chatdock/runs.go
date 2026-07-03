package chatdock

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type MCPRun struct {
	ID         string     `json:"id"`
	Workspace  string     `json:"workspace"`
	SessionID  string     `json:"session_id,omitempty"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	Summary    string     `json:"summary,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMS int64      `json:"duration_ms"`
	EventCount int        `json:"event_count"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type MCPRunEvent struct {
	ID         string     `json:"id"`
	RunID      string     `json:"run_id"`
	Seq        int        `json:"seq"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Server     string     `json:"server,omitempty"`
	Tool       string     `json:"tool,omitempty"`
	Action     string     `json:"action,omitempty"`
	Summary    string     `json:"summary,omitempty"`
	Arguments  any        `json:"arguments,omitempty"`
	Result     any        `json:"result,omitempty"`
	Error      string     `json:"error,omitempty"`
	DurationMS int64      `json:"duration_ms"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type MCPRunResponse struct {
	Runs []MCPRun `json:"runs"`
}

type MCPRunDetailResponse struct {
	Run    MCPRun        `json:"run"`
	Events []MCPRunEvent `json:"events"`
}

type AgentTask struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Workspace string    `json:"workspace"`
	SessionID string    `json:"session_id,omitempty"`
	SourceRun string    `json:"source_run_id"`
	Server    string    `json:"server,omitempty"`
	Action    string    `json:"action,omitempty"`
	Phase     string    `json:"phase,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AgentTaskResponse struct {
	Tasks []AgentTask `json:"tasks"`
}

type runEventInput struct {
	Kind       string
	Status     string
	Tool       string
	Arguments  any
	Result     any
	Error      string
	DurationMS int64
	StartedAt  time.Time
	FinishedAt *time.Time
}

type activeToolRun struct {
	RunID     string
	Created   bool
	LastArgs  map[string]any
	StartedAt map[string]time.Time
}

func (s *Store) StartMCPRun(sessionID string, title string) (MCPRun, error) {
	now := time.Now()
	title = strings.TrimSpace(title)
	if title == "" {
		title = "MCP tool run"
	}
	run := MCPRun{ID: NewID(), Workspace: s.ActivePrompt(), SessionID: strings.TrimSpace(sessionID), Title: title, Status: "running", StartedAt: now, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO mcp_runs(prompt, id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.Workspace, run.ID, run.SessionID, run.Title, run.Status, run.Summary, run.Error, formatDBTime(run.StartedAt), "", run.DurationMS, run.EventCount, formatDBTime(run.UpdatedAt))
	return run, err
}

func (s *Store) AddMCPRunEvent(runID string, input runEventInput) (MCPRunEvent, error) {
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
	server, tool := splitToolFullName(strings.TrimSpace(input.Tool))
	if strings.TrimSpace(input.Tool) == "" {
		server, tool = "", ""
	}
	action := actionFromArguments(input.Arguments)
	event := MCPRunEvent{ID: NewID(), RunID: runID, Kind: firstNonEmptyString(input.Kind, "tool_call"), Status: status, Server: server, Tool: tool, Action: action, Summary: summarizeRunEvent(status, input.Tool, action, input.Error), Arguments: input.Arguments, Result: input.Result, Error: strings.TrimSpace(input.Error), DurationMS: input.DurationMS, StartedAt: started, FinishedAt: input.FinishedAt, CreatedAt: now}
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

func (s *Store) ListMCPRuns(sessionID string, limit int) (MCPRunResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	prompt := s.ActivePrompt()
	sessionID = strings.TrimSpace(sessionID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	query := `SELECT prompt, id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at FROM mcp_runs WHERE prompt = ?`
	args := []any{prompt}
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

func (s *Store) MCPRunDetail(runID string) (MCPRunDetailResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, err := s.getMCPRunLocked(runID)
	if err != nil {
		return MCPRunDetailResponse{}, err
	}
	events, err := s.listMCPRunEventsLocked(runID)
	if err != nil {
		return MCPRunDetailResponse{}, err
	}
	return MCPRunDetailResponse{Run: run, Events: events}, nil
}

func (s *Store) ListAgentTasks(limit int) (AgentTaskResponse, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	prompt := s.ActivePrompt()
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT r.prompt, r.session_id, e.run_id, e.server, e.tool, e.action, e.status, e.summary, e.arguments_json, e.result_json, e.error, e.created_at
FROM mcp_run_events e JOIN mcp_runs r ON r.id = e.run_id
WHERE r.prompt = ? AND e.tool = 'task_manage'
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
		id := firstNonEmptyString(stringFromMap(args, "task_id"), stringFromNestedResult(result, "task_id"), runID)
		if _, exists := tasksByID[id]; exists {
			continue
		}
		title := firstNonEmptyString(stringFromMap(args, "title"), stringFromMap(args, "goal"), stringFromNestedResult(result, "title"), "AgentDock task")
		tasksByID[id] = AgentTask{ID: id, Title: title, Status: agentTaskStatus(action, status, errorText, result), Workspace: workspace, SessionID: sessionID, SourceRun: runID, Server: server, Action: action, Phase: firstNonEmptyString(stringFromNestedResult(result, "phase"), stringFromNestedResult(result, "current_phase")), Summary: firstNonEmptyString(summary, stringFromNestedResult(result, "summary")), Error: errorText, UpdatedAt: parseDBTime(createdRaw)}
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

func (s *Store) getMCPRunLocked(runID string) (MCPRun, error) {
	row := s.db.QueryRow(`SELECT prompt, id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at FROM mcp_runs WHERE id = ?`, strings.TrimSpace(runID))
	rows := &singleRow{scan: row.Scan}
	runs, err := scanMCPRuns(rows)
	if err != nil {
		return MCPRun{}, err
	}
	if len(runs) == 0 {
		return MCPRun{}, sql.ErrNoRows
	}
	return runs[0], nil
}

func (s *Store) listMCPRunEventsLocked(runID string) ([]MCPRunEvent, error) {
	rows, err := s.db.Query(`SELECT id, run_id, seq, kind, status, server, tool, action, summary, arguments_json, result_json, error, duration_ms, started_at, finished_at, created_at FROM mcp_run_events WHERE run_id = ? ORDER BY seq ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []MCPRunEvent
	for rows.Next() {
		event, err := scanMCPRunEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

type runRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

type singleRow struct {
	scan func(dest ...any) error
	done bool
	err  error
}

func (r *singleRow) Next() bool { return !r.done }
func (r *singleRow) Scan(dest ...any) error {
	if r.done {
		return sql.ErrNoRows
	}
	r.done = true
	return r.scan(dest...)
}
func (r *singleRow) Err() error { return r.err }

func scanMCPRuns(rows runRows) ([]MCPRun, error) {
	var runs []MCPRun
	for rows.Next() {
		var run MCPRun
		var startedRaw, finishedRaw, updatedRaw string
		if err := rows.Scan(&run.Workspace, &run.ID, &run.SessionID, &run.Title, &run.Status, &run.Summary, &run.Error, &startedRaw, &finishedRaw, &run.DurationMS, &run.EventCount, &updatedRaw); err != nil {
			if err == sql.ErrNoRows {
				return runs, nil
			}
			return nil, err
		}
		run.StartedAt = parseDBTime(startedRaw)
		if strings.TrimSpace(finishedRaw) != "" {
			finished := parseDBTime(finishedRaw)
			run.FinishedAt = &finished
		}
		run.UpdatedAt = parseDBTime(updatedRaw)
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func scanMCPRunEvent(scan func(dest ...any) error) (MCPRunEvent, error) {
	var event MCPRunEvent
	var argsRaw, resultRaw, startedRaw, finishedRaw, createdRaw string
	err := scan(&event.ID, &event.RunID, &event.Seq, &event.Kind, &event.Status, &event.Server, &event.Tool, &event.Action, &event.Summary, &argsRaw, &resultRaw, &event.Error, &event.DurationMS, &startedRaw, &finishedRaw, &createdRaw)
	if err != nil {
		return MCPRunEvent{}, err
	}
	event.Arguments = decodeJSONValue(argsRaw)
	event.Result = decodeJSONValue(resultRaw)
	event.StartedAt = parseDBTime(startedRaw)
	if strings.TrimSpace(finishedRaw) != "" {
		finished := parseDBTime(finishedRaw)
		event.FinishedAt = &finished
	}
	event.CreatedAt = parseDBTime(createdRaw)
	return event, nil
}

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	result, err := a.store.ListMCPRuns(r.URL.Query().Get("session_id"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.MCPRunDetail(r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == sql.ErrNoRows {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) handleListAgentTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	result, err := a.store.ListAgentTasks(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, result)
}

func (a *App) completeWithRecordedTools(ctx context.Context, sessionID string, cfg ModelConfig, history []Message, emit func(string, any) error) (string, error) {
	mcpCfg, err := a.activeMCPConfig()
	if err != nil || len(mcpCfg.Servers) == 0 {
		if emit != nil {
			return a.client.Stream(ctx, cfg, history, func(delta StreamDelta) error { return emit("delta", delta) })
		}
		return a.client.Complete(ctx, cfg, history)
	}
	tools, err := a.mcpClient.ListTools(ctx, mcpCfg)
	if err != nil || len(tools) == 0 {
		if emit != nil {
			message := "MCP 工具未接入"
			if err != nil {
				message = err.Error()
			}
			if emitErr := emit("tool_setup_error", map[string]any{"message": message}); emitErr != nil {
				return "", emitErr
			}
			return a.client.Stream(ctx, cfg, history, func(delta StreamDelta) error { return emit("delta", delta) })
		}
		return a.client.Complete(ctx, cfg, history)
	}
	if emit != nil {
		if err := emit("tool_setup_ready", map[string]any{"tool_count": len(tools)}); err != nil {
			return "", err
		}
	}
	recorder := &activeToolRun{LastArgs: map[string]any{}, StartedAt: map[string]time.Time{}}
	recordingEmit := func(event string, value any) error {
		if event == "tool_call_start" || event == "tool_call_result" {
			if err := a.recordToolRunEvent(sessionID, recorder, event, value, emit); err != nil {
				return err
			}
		}
		if emit != nil {
			return emit(event, value)
		}
		return nil
	}
	answer, runErr := a.client.CompleteWithMCPToolsEvents(ctx, cfg, history, tools, func(name string, args map[string]any) (any, error) {
		if mcpToolNeedsConfirmation(mcpCfg, name) {
			if err := a.requestMCPConfirmation(ctx, sessionID, name, args, recordingEmit); err != nil {
				return nil, err
			}
			return a.mcpClient.CallToolAfterConfirmation(ctx, mcpCfg, name, args)
		}
		return a.mcpClient.CallTool(ctx, mcpCfg, name, args)
	}, recordingEmit)
	if recorder.Created {
		status := "success"
		if runErr != nil {
			status = "failed"
		}
		run, err := a.store.FinishMCPRun(recorder.RunID, status, "tool run finished", runErr)
		if err == nil && emit != nil {
			_ = emit("run_finish", run)
		}
	}
	return answer, runErr
}

func (a *App) recordToolRunEvent(sessionID string, recorder *activeToolRun, event string, value any, emit func(string, any) error) error {
	data, _ := value.(map[string]any)
	toolName, _ := data["tool"].(string)
	if !recorder.Created {
		run, err := a.store.StartMCPRun(sessionID, "chat tool run")
		if err != nil {
			return err
		}
		recorder.RunID = run.ID
		recorder.Created = true
		if emit != nil {
			_ = emit("run_start", run)
		}
	}
	now := time.Now()
	if event == "tool_call_start" {
		args, _ := data["arguments"].(map[string]any)
		recorder.LastArgs[toolName] = args
		recorder.StartedAt[toolName] = now
		created, err := a.store.AddMCPRunEvent(recorder.RunID, runEventInput{Kind: "tool_call", Status: "running", Tool: toolName, Arguments: args, StartedAt: now})
		if err != nil {
			return err
		}
		if emit != nil {
			_ = emit("run_event", created)
		}
		return nil
	}
	if event != "tool_call_result" {
		return nil
	}
	args := recorder.LastArgs[toolName]
	started := recorder.StartedAt[toolName]
	if started.IsZero() {
		started = now
	}
	status := "success"
	errorText, _ := data["error"].(string)
	if errorText != "" {
		status = "failed"
	}
	finished := now
	created, err := a.store.AddMCPRunEvent(recorder.RunID, runEventInput{Kind: "tool_result", Status: status, Tool: toolName, Arguments: args, Result: data, Error: errorText, StartedAt: started, FinishedAt: &finished, DurationMS: finished.Sub(started).Milliseconds()})
	if err != nil {
		return err
	}
	if emit != nil {
		_ = emit("run_event", created)
	}
	return nil
}

func compactJSONForDB(value any) string {
	if value == nil {
		return "null"
	}
	return compactJSON(value)
}

func decodeJSONValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func decodeJSONMap(raw string) map[string]any {
	value, _ := decodeJSONValue(raw).(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func actionFromArguments(value any) string {
	m, _ := value.(map[string]any)
	return stringFromMap(m, "action")
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func stringFromNestedResult(m map[string]any, key string) string {
	if value := stringFromMap(m, key); value != "" {
		return value
	}
	if nested, _ := m["result"].(map[string]any); nested != nil {
		if value := stringFromMap(nested, key); value != "" {
			return value
		}
	}
	return ""
}

func summarizeRunEvent(status string, toolName string, action string, errorText string) string {
	label := strings.TrimSpace(toolName)
	if action != "" {
		label += " · " + action
	}
	if label == "" {
		label = "tool call"
	}
	if errorText != "" || status == "failed" {
		return "failed: " + label
	}
	if status == "running" {
		return "started: " + label
	}
	return "finished: " + label
}

func agentTaskStatus(action string, status string, errorText string, result map[string]any) string {
	if errorText != "" || status == "failed" {
		return "failed"
	}
	if value := stringFromNestedResult(result, "status"); value != "" {
		return value
	}
	switch action {
	case "complete":
		return "completed"
	case "block":
		return "blocked"
	case "template_match":
		return "matched"
	case "create", "resume", "phase_checkpoint", "complete_step", "record_attempt", "advance":
		return "active"
	default:
		return firstNonEmptyString(status, "active")
	}
}
