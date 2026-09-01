package modelprovider

import (
	"time"

	"chatdock/internal/model"
)

const (
	KeyStrategyAuto   = "auto"
	KeyStrategyManual = "manual"
)

type APIKey struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	HasAPIKey    bool       `json:"has_api_key"`
	APIKeyMasked string     `json:"api_key_masked,omitempty"`
	Enabled      bool       `json:"enabled"`
	Priority     int        `json:"priority"`
	LastStatus   string     `json:"last_status,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Provider struct {
	ID            string                      `json:"id"`
	Name          string                      `json:"name"`
	Type          string                      `json:"type"`
	BaseURL       string                      `json:"base_url"`
	HasAPIKey     bool                        `json:"has_api_key"`
	APIKeyMasked  string                      `json:"api_key_masked,omitempty"`
	DefaultModel  string                      `json:"default_model"`
	Models        []string                    `json:"models,omitempty"`
	ModelLimits   map[string]model.ModelLimit `json:"model_limits,omitempty"`
	TimeoutMS     int                         `json:"timeout_ms"`
	Enabled       bool                        `json:"enabled"`
	KeyStrategy   string                      `json:"key_strategy"`
	SelectedKeyID string                      `json:"selected_key_id,omitempty"`
	APIKeys       []APIKey                    `json:"api_keys,omitempty"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
}

type APIKeyInput struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

type Input struct {
	ID            string                      `json:"id,omitempty"`
	Name          string                      `json:"name"`
	Type          string                      `json:"type,omitempty"`
	BaseURL       string                      `json:"base_url"`
	DefaultModel  string                      `json:"default_model"`
	Models        []string                    `json:"models,omitempty"`
	ModelLimits   map[string]model.ModelLimit `json:"model_limits,omitempty"`
	TimeoutMS     int                         `json:"timeout_ms,omitempty"`
	Enabled       *bool                       `json:"enabled,omitempty"`
	KeyStrategy   string                      `json:"key_strategy,omitempty"`
	SelectedKeyID string                      `json:"selected_key_id,omitempty"`
	APIKeys       []APIKeyInput               `json:"api_keys,omitempty"`
}

type KeyConfig struct {
	KeyID   string
	KeyName string
	Config  model.ModelConfig
}

type APIKeyRecord struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	APIKey       string     `json:"api_key,omitempty"`
	Enabled      bool       `json:"enabled"`
	Priority     int        `json:"priority"`
	LastStatus   string     `json:"last_status,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Record struct {
	ID            string                      `json:"id"`
	Name          string                      `json:"name"`
	Type          string                      `json:"type"`
	BaseURL       string                      `json:"base_url"`
	LegacyAPIKey  string                      `json:"api_key,omitempty"`
	DefaultModel  string                      `json:"default_model"`
	Models        []string                    `json:"models,omitempty"`
	ModelLimits   map[string]model.ModelLimit `json:"model_limits,omitempty"`
	TimeoutMS     int                         `json:"timeout_ms"`
	Enabled       bool                        `json:"enabled"`
	KeyStrategy   string                      `json:"key_strategy,omitempty"`
	SelectedKeyID string                      `json:"selected_key_id,omitempty"`
	APIKeys       []APIKeyRecord              `json:"api_keys,omitempty"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
}
