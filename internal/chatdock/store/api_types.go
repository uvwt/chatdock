package store

import "time"

type SetupStatus struct {
	NeedsSetup       bool   `json:"needs_setup"`
	HasModelProvider bool   `json:"has_model_provider"`
	HasAPIKey        bool   `json:"has_api_key"`
	HasWorkspace     bool   `json:"has_workspace"`
	ActiveWorkspace  string `json:"active_workspace"`
	WorkspaceCount   int    `json:"workspace_count"`
	DataDir          string `json:"data_dir"`
}

type SetupInitRequest struct {
	WorkspaceName string `json:"workspace_name"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	Model         string `json:"model"`
	SystemPrompt  string `json:"system_prompt"`
}

type WorkspaceReadiness struct {
	WorkspaceID      string `json:"workspace_id"`
	Ready            bool   `json:"ready"`
	HasModelProvider bool   `json:"has_model_provider"`
	HasAPIKey        bool   `json:"has_api_key"`
	Model            string `json:"model"`
	ProviderID       string `json:"provider_id"`
	Reason           string `json:"reason,omitempty"`
}

type MCPConfirmationRecord struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	SessionID   string         `json:"session_id,omitempty"`
	Tool        string         `json:"tool"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Status      string         `json:"status"`
	RequestedAt time.Time      `json:"requested_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
	Message     string         `json:"message,omitempty"`
}

type ModelProviderAPIKey struct {
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

type ModelProvider struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Type          string                `json:"type"`
	BaseURL       string                `json:"base_url"`
	HasAPIKey     bool                  `json:"has_api_key"`
	APIKeyMasked  string                `json:"api_key_masked,omitempty"`
	DefaultModel  string                `json:"default_model"`
	Models        []string              `json:"models,omitempty"`
	TimeoutMS     int                   `json:"timeout_ms"`
	Enabled       bool                  `json:"enabled"`
	KeyStrategy   string                `json:"key_strategy"`
	SelectedKeyID string                `json:"selected_key_id,omitempty"`
	APIKeys       []ModelProviderAPIKey `json:"api_keys,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

type ModelProviderAPIKeyInput struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

type ModelProviderInput struct {
	ID            string                     `json:"id,omitempty"`
	Name          string                     `json:"name"`
	Type          string                     `json:"type,omitempty"`
	BaseURL       string                     `json:"base_url"`
	APIKey        string                     `json:"api_key,omitempty"`
	DefaultModel  string                     `json:"default_model"`
	Models        []string                   `json:"models,omitempty"`
	TimeoutMS     int                        `json:"timeout_ms,omitempty"`
	Enabled       bool                       `json:"enabled"`
	KeyStrategy   string                     `json:"key_strategy,omitempty"`
	SelectedKeyID string                     `json:"selected_key_id,omitempty"`
	APIKeys       []ModelProviderAPIKeyInput `json:"api_keys,omitempty"`
}

type Workspace struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Icon            string    `json:"icon,omitempty"`
	ProviderID      string    `json:"provider_id"`
	Model           string    `json:"model"`
	Models          []string  `json:"models,omitempty"`
	SystemPrompt    string    `json:"system_prompt"`
	ContextLimit    int       `json:"context_limit"`
	Temperature     float64   `json:"temperature"`
	HideThinking    bool      `json:"hide_thinking"`
	EnableReasoning bool      `json:"enable_reasoning"`
	Ready           bool      `json:"ready"`
	ReadinessReason string    `json:"readiness_reason,omitempty"`
	TaskCount       int       `json:"task_count"`
	SessionCount    int       `json:"session_count"`
	Active          bool      `json:"active"`
	Archived        bool      `json:"archived"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type WorkspaceResponse struct {
	Active     string      `json:"active"`
	Workspaces []Workspace `json:"workspaces"`
}

type PromptPreviewResponse struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Content       string `json:"content"`
}

type BackupInfo struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"size_bytes"`
	UpdatedAt  time.Time `json:"updated_at"`
	AgeSeconds int64     `json:"age_seconds"`
}

type DataStatus struct {
	DataDir                string       `json:"data_dir"`
	DatabasePath           string       `json:"database_path"`
	DatabaseExists         bool         `json:"database_exists"`
	DatabaseHealthy        bool         `json:"database_healthy"`
	DatabaseCheck          string       `json:"database_check,omitempty"`
	DatabaseWarning        string       `json:"database_warning,omitempty"`
	DatabaseSizeBytes      int64        `json:"database_size_bytes"`
	WALEnabled             bool         `json:"wal_enabled"`
	SHMExists              bool         `json:"shm_exists"`
	BackupDir              string       `json:"backup_dir,omitempty"`
	BackupCheckedDirs      []string     `json:"backup_checked_dirs,omitempty"`
	BackupCount            int          `json:"backup_count"`
	BackupHealthy          bool         `json:"backup_healthy"`
	BackupWarning          string       `json:"backup_warning,omitempty"`
	LatestBackupPath       string       `json:"latest_backup_path,omitempty"`
	LatestBackupSizeBytes  int64        `json:"latest_backup_size_bytes,omitempty"`
	LatestBackupAt         time.Time    `json:"latest_backup_at,omitempty"`
	LatestBackupAgeSeconds int64        `json:"latest_backup_age_seconds,omitempty"`
	Backups                []BackupInfo `json:"backups,omitempty"`
	ActiveWorkspace        string       `json:"active_workspace"`
	WorkspaceCount         int          `json:"workspace_count"`
	SessionCount           int          `json:"session_count"`
}

type MCPServerStatus struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Disabled     bool   `json:"disabled"`
	AuthType     string `json:"auth_type,omitempty"`
	HasToken     bool   `json:"has_token"`
	AllowCount   int    `json:"allow_count"`
	DenyCount    int    `json:"deny_count"`
	ConfirmCount int    `json:"confirm_count"`
	TimeoutMS    int    `json:"timeout_ms"`
	CacheTTLMS   int    `json:"cache_ttl_ms"`
	LastStatus   string `json:"last_status"`
	LastError    string `json:"last_error,omitempty"`
}

type MCPStatusResponse struct {
	Servers []MCPServerStatus `json:"servers"`
}
