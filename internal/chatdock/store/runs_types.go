package store

import "time"

type MCPRun struct {
	ID         string     `json:"id"`
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

type RunEventInput struct {
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
