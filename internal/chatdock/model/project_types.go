package model

import "time"

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Projects []Project `json:"projects"`
}

type PromptPreviewResponse struct {
	ProjectID string `json:"project_id,omitempty"`
	Content   string `json:"content"`
}
