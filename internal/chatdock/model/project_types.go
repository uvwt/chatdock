package model

import "time"

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProjectSummary struct {
	Project
	SessionCount int `json:"session_count"`
}

type CreateProjectRequest struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type UpdateProjectRequest struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type ProjectListResponse struct {
	Projects          []ProjectSummary `json:"projects"`
	SessionCount      int              `json:"session_count"`
	PlainSessionCount int              `json:"plain_session_count"`
}

type PromptPreviewResponse struct {
	ProjectID string `json:"project_id,omitempty"`
	Content   string `json:"content"`
}
