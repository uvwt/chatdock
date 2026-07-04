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
	EmbeddingBaseURL   string   `json:"embedding_base_url,omitempty"`
	EmbeddingAPIKey    string   `json:"embedding_api_key,omitempty"`
	EmbeddingModel     string   `json:"embedding_model,omitempty"`
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
	EmbeddingBaseURL   string   `json:"embedding_base_url,omitempty"`
	HasEmbeddingAPIKey bool     `json:"has_embedding_api_key"`
	EmbeddingModel     string   `json:"embedding_model,omitempty"`
}
