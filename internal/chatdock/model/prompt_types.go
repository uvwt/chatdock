package model

import "time"

type PromptSpace struct {
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Count     int       `json:"count"`
}

type CreatePromptRequest struct {
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
}

type SelectPromptRequest struct {
	Name string `json:"name"`
}

type PromptResponse struct {
	Active  string        `json:"active"`
	Prompts []PromptSpace `json:"prompts"`
}
