package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"chatdock/internal/llm"
	"chatdock/internal/mcp"
)

func (s *Store) getMCPRunLocked(runID string) (MCPRun, error) {
	row := s.db.QueryRow(`SELECT id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at FROM mcp_runs WHERE id = ?`, strings.TrimSpace(runID))
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
		if err := rows.Scan(&run.ID, &run.SessionID, &run.Title, &run.Status, &run.Summary, &run.Error, &startedRaw, &finishedRaw, &run.DurationMS, &run.EventCount, &updatedRaw); err != nil {
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

func compactJSONForDB(value any) string {
	if value == nil {
		return "null"
	}
	return mcp.CompactJSON(value)
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
	case "match":
		return "matched"
	case "create", "resume", "phase_checkpoint", "complete_step", "record_attempt", "advance":
		return "active"
	default:
		return llm.FirstNonEmptyString(status, "active")
	}
}
