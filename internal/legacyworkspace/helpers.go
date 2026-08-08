package legacyworkspace

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"chatdock/internal/model"
	"chatdock/internal/modelprovider"
)

const (
	privateDirMode  = 0o700
	privateFileMode = 0o600
)

type sqlWriter interface {
	Exec(string, ...any) (sql.Result, error)
}
type sqlQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}
type sqlQueryWriter interface {
	sqlWriter
	sqlQueryer
}

func setMetaValueWith(writer sqlWriter, key, value string) error {
	_, err := writer.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func getGlobalRawWith(reader sqlQueryer, key string) (string, bool, error) {
	var value string
	err := reader.QueryRow(`SELECT value FROM global_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func setGlobalJSONWith(writer sqlWriter, key string, value any, now time.Time) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = writer.Exec(`INSERT INTO global_settings(key, value, updated_at) VALUES(?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, string(raw)+"\n", now.Format(time.RFC3339Nano))
	return err
}

func sqliteTableExists(q interface{ QueryRow(string, ...any) *sql.Row }, name string) (bool, error) {
	var got string
	err := q.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func sqliteColumnExists(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, table, column string) (bool, error) {
	rows, err := q.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func rejectLegacyWorkspaceSchema(db *sql.DB) error {
	for _, table := range []string{"workspaces", "workspace_kv", "prompts", "prompt_kv"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("legacy workspace schema detected: table %s exists", table)
		}
	}
	for _, table := range []string{"sessions", "session_messages", "session_message_parts", "session_message_events", "session_message_event_details", "mcp_runs", "mcp_confirmations", "chat_jobs", "scheduled_tasks", "scheduled_task_runs", "attachments", "tool_embeddings"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		hasWorkspace, err := sqliteColumnExists(db, table, "workspace_id")
		if err != nil {
			return err
		}
		if hasWorkspace {
			return fmt.Errorf("legacy workspace schema detected: column %s.workspace_id exists", table)
		}
		hasPrompt, err := sqliteColumnExists(db, table, "prompt")
		if err != nil {
			return err
		}
		if hasPrompt {
			return fmt.Errorf("legacy prompt schema detected: column %s.prompt exists", table)
		}
	}
	return nil
}

func modelConfigWith(reader sqlQueryer) (model.ModelConfig, error) {
	raw, ok, err := getGlobalRawWith(reader, "config")
	if err != nil {
		return model.ModelConfig{}, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return model.DefaultModelConfig(), nil
	}
	var cfg model.ModelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return model.ModelConfig{}, err
	}
	cfg = model.NormalizeModelConfig(cfg)
	records, err := modelprovider.LoadRecords(reader)
	if err != nil {
		return model.ModelConfig{}, err
	}
	for _, record := range records {
		if record.ID != modelprovider.NormalizeID(cfg.ProviderID) {
			continue
		}
		apiKey, err := modelprovider.SelectedAPIKey(record)
		if err != nil {
			return model.ModelConfig{}, err
		}
		cfg.ProviderID = record.ID
		cfg.BaseURL = record.BaseURL
		cfg.APIKey = apiKey
		cfg.Models = append([]string(nil), record.Models...)
		if cfg.Model == "" {
			cfg.Model = record.DefaultModel
		}
		break
	}
	return model.NormalizeModelConfig(cfg), nil
}

func setGlobalRawWith(writer sqlWriter, key, value string, now time.Time) error {
	_, err := writer.Exec(`INSERT INTO global_settings(key, value, updated_at) VALUES(?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now.Format(time.RFC3339Nano))
	return err
}

func modelProviderConfigWith(reader sqlQueryer, id string) (model.ModelConfig, bool, error) {
	id = modelprovider.NormalizeID(id)
	if id == "" {
		return model.ModelConfig{}, false, nil
	}
	records, err := modelprovider.LoadRecords(reader)
	if err != nil {
		return model.ModelConfig{}, false, err
	}
	for _, record := range records {
		if record.ID != id {
			continue
		}
		apiKey, err := modelprovider.SelectedAPIKey(record)
		if err != nil {
			return model.ModelConfig{}, true, err
		}
		return model.NormalizeModelConfig(model.ModelConfig{
			ProviderID: record.ID,
			BaseURL:    record.BaseURL,
			APIKey:     apiKey,
			Model:      record.DefaultModel,
			Models:     append([]string(nil), record.Models...),
		}), true, nil
	}
	return model.ModelConfig{}, false, nil
}

func applyProviderToConfigWith(reader sqlQueryer, cfg model.ModelConfig) (model.ModelConfig, error) {
	cfg = model.NormalizeModelConfig(cfg)
	providerCfg, ok, err := modelProviderConfigWith(reader, cfg.ProviderID)
	if err != nil {
		return model.ModelConfig{}, err
	}
	if !ok {
		return cfg, nil
	}
	cfg.ProviderID = providerCfg.ProviderID
	cfg.BaseURL = providerCfg.BaseURL
	cfg.APIKey = providerCfg.APIKey
	cfg.Models = append([]string(nil), providerCfg.Models...)
	if cfg.Model == "" {
		cfg.Model = providerCfg.Model
	}
	return model.NormalizeModelConfig(cfg), nil
}

func normalizeFallbackModelSelectionWith(reader sqlQueryer, cfg *model.ModelConfig) error {
	providerID := modelprovider.NormalizeID(cfg.FallbackProviderID)
	if providerID == "" {
		cfg.FallbackProviderID = ""
		cfg.FallbackModel = ""
		return nil
	}
	providerCfg, ok, err := modelProviderConfigWith(reader, providerID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("fallback model provider not found: %s", providerID)
	}
	modelName := strings.TrimSpace(cfg.FallbackModel)
	if modelName == "" {
		modelName = providerCfg.Model
	}
	if modelName == "" {
		return fmt.Errorf("fallback model is required")
	}
	if providerCfg.ProviderID == cfg.ProviderID && modelName == cfg.Model {
		return fmt.Errorf("fallback model must differ from primary model")
	}
	cfg.FallbackProviderID = providerCfg.ProviderID
	cfg.FallbackModel = modelName
	return nil
}

func upsertProviderFromConfigWith(db sqlQueryWriter, cfg model.ModelConfig) error {
	records, err := modelprovider.LoadRecords(db)
	if err != nil {
		return err
	}
	now := time.Now()
	id := modelprovider.NormalizeID(cfg.ProviderID)
	if id == "" {
		id = "provider_default"
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		records[i].BaseURL = strings.TrimSpace(cfg.BaseURL)
		records[i].APIKeys = modelprovider.UpsertAPIKey(records[i].APIKeys, records[i].SelectedKeyID, cfg.APIKey, now)
		records[i].DefaultModel = strings.TrimSpace(cfg.Model)
		records[i].Models = modelprovider.NormalizeModelNames(cfg.Models, cfg.Model)
		records[i].Enabled = strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != ""
		records[i].UpdatedAt = now
		records[i] = modelprovider.NormalizeRecord(records[i])
		return modelprovider.SaveRecords(db, records)
	}
	record := modelprovider.NormalizeRecord(modelprovider.Record{
		ID: id, Name: modelprovider.DisplayName(cfg), Type: "openai-compatible", BaseURL: strings.TrimSpace(cfg.BaseURL),
		APIKeys: modelprovider.UpsertAPIKey(nil, "", cfg.APIKey, now), DefaultModel: strings.TrimSpace(cfg.Model),
		Models: modelprovider.NormalizeModelNames(cfg.Models, cfg.Model), TimeoutMS: 120000,
		Enabled: strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "", CreatedAt: now, UpdatedAt: now,
	})
	return modelprovider.SaveRecords(db, append(records, record))
}

func parseDBTimeZero(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	return time.Time{}
}

func normalizeProjectID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("project id is empty")
	}
	if !utf8.ValidString(id) {
		return "", fmt.Errorf("project id is invalid")
	}
	if utf8.RuneCountInString(id) > 128 {
		return "", fmt.Errorf("project id exceeds 128 characters")
	}
	if id == "." || id == ".." || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return "", fmt.Errorf("project id cannot contain path separators")
	}
	for _, r := range id {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("project id contains control characters")
		}
	}
	return id, nil
}

func normalizeProjectName(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\n", " "))
	if name == "" {
		return "", fmt.Errorf("project name is empty")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("project name is invalid")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("project name contains control characters")
		}
	}
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	return name, nil
}
