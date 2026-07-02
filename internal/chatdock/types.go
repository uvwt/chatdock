package chatdock

import "time"

type ServerConfig struct {
	Addr           string
	DataDir        string
	WebDir         string
	AuthToken      string
	AuthUsername   string
	AuthCredential string
}

type AuthStatusResponse struct {
	Enabled      bool   `json:"enabled"`
	LoginEnabled bool   `json:"login_enabled"`
	Username     string `json:"username,omitempty"`
}

type AuthLoginRequest struct {
	Username   string `json:"username"`
	Credential string `json:"credential"`
}

type AuthLoginResponse struct {
	OK       bool   `json:"ok"`
	Token    string `json:"token"`
	Username string `json:"username,omitempty"`
}

type ModelConfig struct {
	ProviderID         string  `json:"provider_id,omitempty"`
	BaseURL            string  `json:"base_url"`
	APIKey             string  `json:"api_key,omitempty"`
	Model              string  `json:"model"`
	SystemPrompt       string  `json:"system_prompt"`
	Skills             []Skill `json:"-"`
	MaxContextMessages int     `json:"max_context_messages"`
	Temperature        float64 `json:"temperature"`
	EnableThinking     bool    `json:"enable_thinking"`
	HideThinking       bool    `json:"hide_thinking"`
}

type PublicModelConfig struct {
	ProviderID         string  `json:"provider_id,omitempty"`
	BaseURL            string  `json:"base_url"`
	HasAPIKey          bool    `json:"has_api_key"`
	Model              string  `json:"model"`
	SystemPrompt       string  `json:"system_prompt"`
	MaxContextMessages int     `json:"max_context_messages"`
	Temperature        float64 `json:"temperature"`
	EnableThinking     bool    `json:"enable_thinking"`
	HideThinking       bool    `json:"hide_thinking"`
}

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

type MCPConfigResponse struct {
	Content string `json:"content"`
}

type SaveMCPConfigRequest struct {
	Content string `json:"content"`
}

type MCPToolsResponse struct {
	Tools []MCPTool `json:"tools"`
}

type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SaveSkillRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Enabled     bool   `json:"enabled"`
}

type SkillResponse struct {
	Skills []Skill `json:"skills"`
}

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

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Reasoning string    `json:"reasoning,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Pinned    bool      `json:"pinned"`
	Preview   string    `json:"preview,omitempty"`
	LastRole  string    `json:"last_role,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Count     int       `json:"count"`
}

type RenameSessionRequest struct {
	Title string `json:"title"`
}

type PinSessionRequest struct {
	Pinned bool `json:"pinned"`
}

type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type ChatResponse struct {
	Answer  string   `json:"answer"`
	Session *Session `json:"session"`
}
