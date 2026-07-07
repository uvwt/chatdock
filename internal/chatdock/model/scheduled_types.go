package model

import "time"

const (
	ScheduledTaskContextStateless  = "stateless"
	ScheduledTaskContextLastResult = "last_result"
	ScheduledTaskContextSession    = "session"
)

type ScheduledTask struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Prompt          string     `json:"prompt"`
	Enabled         bool       `json:"enabled"`
	ScheduleType    string     `json:"schedule_type"`
	RunAt           *time.Time `json:"run_at,omitempty"`
	TimeOfDay       string     `json:"time_of_day,omitempty"`
	IntervalMinutes int        `json:"interval_minutes,omitempty"`
	ContextMode     string     `json:"context_mode,omitempty"`
	NextRunAt       time.Time  `json:"next_run_at"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastStatus      string     `json:"last_status,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	SessionID       string     `json:"session_id,omitempty"`
	Running         bool       `json:"running"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ScheduledTaskRequest struct {
	Title           string `json:"title"`
	Prompt          string `json:"prompt"`
	Enabled         bool   `json:"enabled"`
	ScheduleType    string `json:"schedule_type"`
	RunAt           string `json:"run_at"`
	TimeOfDay       string `json:"time_of_day"`
	IntervalMinutes int    `json:"interval_minutes"`
	ContextMode     string `json:"context_mode"`
	Reschedule      bool   `json:"reschedule"`
}

type ScheduledTaskResponse struct {
	Tasks []ScheduledTask `json:"tasks"`
}

type ScheduledTaskRun struct {
	Task        ScheduledTask
	WorkspaceID string
	SessionID   string
	RunID       string
	Config      ModelConfig
	History     []Message
}

type ScheduledTaskRunResponse struct {
	Task    ScheduledTask           `json:"task"`
	Session *Session                `json:"session,omitempty"`
	Run     *ScheduledTaskRunRecord `json:"run,omitempty"`
}

type ScheduledTaskRunRecord struct {
	ID         string     `json:"id"`
	TaskID     string     `json:"task_id"`
	TaskTitle  string     `json:"task_title,omitempty"`
	Prompt     string     `json:"prompt,omitempty"`
	Output     string     `json:"output,omitempty"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	Manual     bool       `json:"manual"`
	SessionID  string     `json:"session_id,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMS int64      `json:"duration_ms,omitempty"`
}

type ScheduledTaskRunRecordResponse struct {
	Runs []ScheduledTaskRunRecord `json:"runs"`
}
