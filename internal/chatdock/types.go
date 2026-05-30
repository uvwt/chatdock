package chatdock

import "time"

type ServerConfig struct {
	Addr    string
	DataDir string
	WebDir  string
}

type ModelConfig struct {
	BaseURL            string  `json:"base_url"`
	APIKey             string  `json:"api_key,omitempty"`
	Model              string  `json:"model"`
	SystemPrompt       string  `json:"system_prompt"`
	MaxContextMessages int     `json:"max_context_messages"`
	Temperature        float64 `json:"temperature"`
}

type PublicModelConfig struct {
	BaseURL            string  `json:"base_url"`
	HasAPIKey          bool    `json:"has_api_key"`
	Model              string  `json:"model"`
	SystemPrompt       string  `json:"system_prompt"`
	MaxContextMessages int     `json:"max_context_messages"`
	Temperature        float64 `json:"temperature"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Count     int       `json:"count"`
}

type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type ChatResponse struct {
	Answer  string   `json:"answer"`
	Session *Session `json:"session"`
}
