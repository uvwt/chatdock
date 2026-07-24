package model

import "strings"

const (
	ContextModeAuto         = "auto"
	ContextModeCompact      = "compact"
	ContextModeExpanded     = "expanded"
	ContextModeCustom       = "custom"
	MaxContextMessagesLimit = 200
)

func DefaultModelConfig() ModelConfig {
	return ModelConfig{
		ProviderID:         "provider_default",
		BaseURL:            "https://api.openai.com/v1",
		Model:              "gpt-4o-mini",
		Models:             []string{"gpt-4o-mini"},
		SystemPrompt:       "你是 ChatDock，一个简洁、直接、节省 token 的私人 AI 助手。默认用中文回答。",
		ContextMode:        ContextModeAuto,
		MaxContextMessages: 12,
		Temperature:        0.7,
		HideThinking:       false,
		EmbeddingModel:     "BAAI/bge-m3",
	}
}

func NormalizeModelConfig(cfg ModelConfig) ModelConfig {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.ProviderID = strings.TrimSpace(cfg.ProviderID)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.FallbackProviderID = strings.TrimSpace(cfg.FallbackProviderID)
	cfg.FallbackModel = strings.TrimSpace(cfg.FallbackModel)
	cfg.EmbeddingBaseURL = strings.TrimSpace(cfg.EmbeddingBaseURL)
	cfg.EmbeddingAPIKey = strings.TrimSpace(cfg.EmbeddingAPIKey)
	cfg.EmbeddingModel = strings.TrimSpace(cfg.EmbeddingModel)
	cfg.Models = normalizeModelNames(cfg.Models, cfg.Model)
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
	if cfg.EmbeddingModel == "" {
		cfg.EmbeddingModel = "BAAI/bge-m3"
	}
	if len(cfg.Models) == 0 {
		cfg.Models = []string{cfg.Model}
	}
	if !containsModelName(cfg.Models, cfg.Model) {
		cfg.Models = append([]string{cfg.Model}, cfg.Models...)
	}
	if cfg.MaxContextMessages <= 0 {
		cfg.MaxContextMessages = 12
	} else if cfg.MaxContextMessages > MaxContextMessagesLimit {
		cfg.MaxContextMessages = MaxContextMessagesLimit
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
		Models:             append([]string(nil), cfg.Models...),
		FallbackProviderID: cfg.FallbackProviderID,
		FallbackModel:      cfg.FallbackModel,
		SystemPrompt:       cfg.SystemPrompt,
		ContextMode:        cfg.ContextMode,
		MaxContextMessages: cfg.MaxContextMessages,
		Temperature:        cfg.Temperature,
		HideThinking:       cfg.HideThinking,
		EmbeddingBaseURL:   cfg.EmbeddingBaseURL,
		HasEmbeddingAPIKey: strings.TrimSpace(cfg.EmbeddingAPIKey) != "",
		EmbeddingModel:     cfg.EmbeddingModel,
	}
}

func normalizeModelNames(models []string, selected string) []string {
	out := make([]string, 0, len(models)+1)
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, model := range models {
		add(model)
	}
	add(selected)
	return out
}

func containsModelName(models []string, selected string) bool {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return false
	}
	for _, model := range models {
		if strings.TrimSpace(model) == selected {
			return true
		}
	}
	return false
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
