package model

import "time"

type WorkspaceSummary struct {
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Count     int       `json:"count"`
}

type CreateWorkspaceRequest struct {
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
}

type WorkspaceIDRequest struct {
	Name string `json:"name"`
}

type WorkspaceListResponse struct {
	Active     string             `json:"active"`
	Workspaces []WorkspaceSummary `json:"workspaces"`
}
