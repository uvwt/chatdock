package model

import "time"

type ScheduledTask struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Prompt          string     `json:"prompt"`
	Enabled         bool       `json:"enabled"`
	ScheduleType    string     `json:"schedule_type"`
	RunAt           *time.Time `json:"run_at,omitempty"`
	TimeOfDay       string     `json:"time_of_day,omitempty"`
	IntervalMinutes int        `json:"interval_minutes,omitempty"`
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
}

type ScheduledTaskResponse struct {
	Tasks []ScheduledTask `json:"tasks"`
}

type ScheduledTaskRun struct {
	Task       ScheduledTask
	PromptName string
	SessionID  string
	Config     ModelConfig
	History    []Message
}

type ScheduledTaskRunResponse struct {
	Task    ScheduledTask `json:"task"`
	Session *Session      `json:"session,omitempty"`
}
