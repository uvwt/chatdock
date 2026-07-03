package model

type ModelConfig struct {
	ProviderID         string   `json:"provider_id,omitempty"`
	BaseURL            string   `json:"base_url"`
	APIKey             string   `json:"api_key,omitempty"`
	Model              string   `json:"model"`
	Models             []string `json:"models,omitempty"`
	SystemPrompt       string   `json:"system_prompt"`
	Skills             []Skill  `json:"-"`
	ContextMode        string   `json:"context_mode,omitempty"`
	MaxContextMessages int      `json:"max_context_messages"`
	Temperature        float64  `json:"temperature"`
	EnableThinking     bool     `json:"enable_thinking"`
	HideThinking       bool     `json:"hide_thinking"`
}

type PublicModelConfig struct {
	ProviderID         string   `json:"provider_id,omitempty"`
	BaseURL            string   `json:"base_url"`
	HasAPIKey          bool     `json:"has_api_key"`
	Model              string   `json:"model"`
	Models             []string `json:"models,omitempty"`
	SystemPrompt       string   `json:"system_prompt"`
	ContextMode        string   `json:"context_mode,omitempty"`
	MaxContextMessages int      `json:"max_context_messages"`
	Temperature        float64  `json:"temperature"`
	EnableThinking     bool     `json:"enable_thinking"`
	HideThinking       bool     `json:"hide_thinking"`
}
