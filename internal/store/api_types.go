package store

import (
	"time"

	"chatdock/internal/modelprovider"
)

type SetupStatus struct {
	NeedsSetup       bool   `json:"needs_setup"`
	HasModelProvider bool   `json:"has_model_provider"`
	HasAPIKey        bool   `json:"has_api_key"`
	ProjectCount     int    `json:"project_count"`
	DataDir          string `json:"data_dir"`
}

type SetupInitRequest struct {
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}

type MCPConfirmationRecord struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id,omitempty"`
	Tool        string         `json:"tool"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Status      string         `json:"status"`
	RequestedAt time.Time      `json:"requested_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
	Message     string         `json:"message,omitempty"`
}

type ModelProviderAPIKey = modelprovider.APIKey
type ModelProvider = modelprovider.Provider
type ModelProviderAPIKeyInput = modelprovider.APIKeyInput
type ModelProviderInput = modelprovider.Input

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
	ProjectCount           int          `json:"project_count"`
	SessionCount           int          `json:"session_count"`
}

type SystemStatusResponse struct {
	OK                       bool        `json:"ok"`
	Name                     string      `json:"name"`
	Time                     time.Time   `json:"time"`
	Setup                    SetupStatus `json:"setup"`
	Data                     DataStatus  `json:"data"`
	WebDir                   string      `json:"web_dir"`
	Addr                     string      `json:"addr"`
	Database                 string      `json:"database"`
	AgentDockTasksConfigured bool        `json:"agentdock_tasks_configured"`
}

type MCPServerStatus struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Description  string `json:"description,omitempty"`
	ToolExposure string `json:"tool_exposure"`
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
