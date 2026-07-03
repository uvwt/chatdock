package chatdock

import "strings"

const (
	ContextModeAuto     = "auto"
	ContextModeCompact  = "compact"
	ContextModeExpanded = "expanded"
	ContextModeCustom   = "custom"
)

func DefaultModelConfig() ModelConfig {
	return ModelConfig{
		ProviderID:         "provider_default",
		BaseURL:            "https://api.openai.com/v1",
		Model:              "gpt-4o-mini",
		SystemPrompt:       "你是 ChatDock，一个简洁、直接、节省 token 的私人 AI 助手。默认用中文回答。",
		ContextMode:        ContextModeAuto,
		MaxContextMessages: 12,
		Temperature:        0.7,
		EnableThinking:     false,
		HideThinking:       true,
	}
}

func NormalizeModelConfig(cfg ModelConfig) ModelConfig {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.ProviderID = strings.TrimSpace(cfg.ProviderID)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.ContextMode = normalizeContextMode(cfg.ContextMode)

	if cfg.ProviderID == "" {
		cfg.ProviderID = "provider_default"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.MaxContextMessages <= 0 {
		cfg.MaxContextMessages = 12
	}
	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		cfg.Temperature = 0.7
	}
	return cfg
}

func ToPublicModelConfig(cfg ModelConfig) PublicModelConfig {
	return PublicModelConfig{
		ProviderID:         cfg.ProviderID,
		BaseURL:            cfg.BaseURL,
		HasAPIKey:          strings.TrimSpace(cfg.APIKey) != "",
		Model:              cfg.Model,
		SystemPrompt:       cfg.SystemPrompt,
		ContextMode:        cfg.ContextMode,
		MaxContextMessages: cfg.MaxContextMessages,
		Temperature:        cfg.Temperature,
		EnableThinking:     cfg.EnableThinking,
		HideThinking:       cfg.HideThinking,
	}
}

func normalizeContextMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ContextModeCompact:
		return ContextModeCompact
	case ContextModeExpanded:
		return ContextModeExpanded
	case ContextModeCustom:
		return ContextModeCustom
	default:
		return ContextModeAuto
	}
}
