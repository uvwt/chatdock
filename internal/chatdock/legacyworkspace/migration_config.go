package legacyworkspace

import (
	"chatdock/internal/chatdock/modelprovider"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
)

func loadLegacyWorkspaceRecords(reader sqlQueryer) ([]legacyWorkspaceRecord, error) {
	rows, err := reader.Query(`SELECT name, created_at, updated_at FROM workspaces ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workspaces []legacyWorkspaceRecord
	for rows.Next() {
		var workspace legacyWorkspaceRecord
		if err := rows.Scan(&workspace.Name, &workspace.CreatedAt, &workspace.UpdatedAt); err != nil {
			return nil, err
		}
		configRaw, ok, err := legacyWorkspaceValue(reader, workspace.Name, "config")
		if err != nil {
			return nil, err
		}
		if ok && strings.TrimSpace(configRaw) != "" {
			if err := json.Unmarshal([]byte(configRaw), &workspace.Config); err != nil {
				return nil, fmt.Errorf("workspace %s config: %w", workspace.Name, err)
			}
			workspace.ConfigRaw = configRaw
		}
		mcpRaw, ok, err := legacyWorkspaceValue(reader, workspace.Name, "mcp")
		if err != nil {
			return nil, err
		}
		if !ok || strings.TrimSpace(mcpRaw) == "" {
			mcpRaw = mcp.DefaultMCPConfig()
		}
		workspace.MCPConfig, err = mcp.ParseMCPConfig(mcpRaw)
		if err != nil {
			return nil, fmt.Errorf("workspace %s mcp config: %w", workspace.Name, err)
		}
		workspace.MCPRaw = mcpRaw
		if err := reader.QueryRow(`SELECT COUNT(*) FROM sessions WHERE workspace_id = ?`, workspace.Name).Scan(&workspace.Sessions); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(workspaces) == 0 {
		return nil, fmt.Errorf("legacy database contains no workspaces")
	}
	return workspaces, nil
}

func legacyWorkspaceValue(reader sqlQueryer, workspace string, key string) (string, bool, error) {
	var value string
	err := reader.QueryRow(`SELECT value FROM workspace_kv WHERE workspace_id = ? AND key = ?`, workspace, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func resolveLegacyWorkspaceProviders(reader sqlQueryer, plan legacyWorkspaceMigrationPlan, records []modelprovider.Record) (map[string]LegacyWorkspaceMigrationProvider, []modelprovider.Record, []LegacyWorkspaceMigrationProvider, error) {
	mapping := map[string]LegacyWorkspaceMigrationProvider{}
	var reports []LegacyWorkspaceMigrationProvider
	for _, workspace := range plan.Workspaces {
		if workspace.Name == plan.GlobalWorkspace {
			continue
		}
		var needsProvider int64
		if err := reader.QueryRow(`SELECT COUNT(*) FROM sessions WHERE workspace_id = ? AND (provider_id = '' OR model = '')`, workspace.Name).Scan(&needsProvider); err != nil {
			return nil, nil, nil, err
		}
		if needsProvider == 0 {
			continue
		}
		if strings.TrimSpace(workspace.ConfigRaw) == "" || strings.TrimSpace(workspace.Config.BaseURL) == "" || strings.TrimSpace(workspace.Config.Model) == "" {
			return nil, nil, nil, fmt.Errorf("workspace %s has sessions without model selection but no complete model config", workspace.Name)
		}
		cfg := model.NormalizeModelConfig(workspace.Config)

		providerID, nextRecords, created, err := ensureLegacyWorkspaceProvider(records, workspace, cfg)
		if err != nil {
			return nil, nil, nil, err
		}
		records = nextRecords
		item := LegacyWorkspaceMigrationProvider{Workspace: workspace.Name, ProviderID: providerID, Model: cfg.Model}
		mapping[workspace.Name] = item
		if created {
			reports = append(reports, item)
		}
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Workspace < reports[j].Workspace })
	return mapping, records, reports, nil
}

func ensureLegacyWorkspaceProvider(records []modelprovider.Record, workspace legacyWorkspaceRecord, cfg model.ModelConfig) (string, []modelprovider.Record, bool, error) {
	desiredID := modelprovider.NormalizeID(cfg.ProviderID)
	if desiredID == "" {
		desiredID = "legacy-" + modelprovider.NormalizeID(workspace.Name)
	}
	for i := range records {
		if records[i].ID != desiredID {
			continue
		}
		selectedKey, err := modelprovider.SelectedAPIKey(records[i])
		if err != nil {
			return "", nil, false, err
		}
		sameEndpoint := strings.TrimRight(strings.TrimSpace(records[i].BaseURL), "/") == strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
		sameCredential := strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APIKey) == strings.TrimSpace(selectedKey)
		if sameEndpoint && sameCredential {
			records[i].Models = modelprovider.NormalizeModelNames(records[i].Models, cfg.Model)
			records[i].UpdatedAt = time.Now()
			records[i] = modelprovider.NormalizeRecord(records[i])
			return records[i].ID, records, false, nil
		}
		break
	}

	baseID := "legacy-" + modelprovider.NormalizeID(workspace.Name)
	if baseID == "legacy-" {
		baseID = "legacy-workspace"
	}
	providerID := modelprovider.UniqueID(baseID, records)
	createdAt := parseDBTimeZero(workspace.CreatedAt)
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	now := time.Now()
	record := modelprovider.NormalizeRecord(modelprovider.Record{
		ID:           providerID,
		Name:         workspace.Name + "（旧工作区）",
		Type:         "openai-compatible",
		BaseURL:      strings.TrimSpace(cfg.BaseURL),
		APIKeys:      modelprovider.UpsertAPIKey(nil, "", cfg.APIKey, now),
		DefaultModel: strings.TrimSpace(cfg.Model),
		Models:       modelprovider.NormalizeModelNames(cfg.Models, cfg.Model),
		TimeoutMS:    120000,
		Enabled:      strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "",
		CreatedAt:    createdAt,
		UpdatedAt:    now,
	})
	records = append(records, record)
	return providerID, records, true, nil
}

func mergeLegacyWorkspaceMCPConfigs(workspaces []legacyWorkspaceRecord, globalWorkspace string) (mcp.MCPConfig, []string, []string, error) {
	var merged mcp.MCPConfig
	foundGlobal := false
	for _, workspace := range workspaces {
		if workspace.Name == globalWorkspace {
			// MCPConfig 内部包含 map，直接赋值会污染迁移计划里的原始工作空间配置，
			// 导致第二次生成报告时把刚合并的 Server 误判为原本就存在。
			raw, err := json.Marshal(workspace.MCPConfig)
			if err != nil {
				return mcp.MCPConfig{}, nil, nil, err
			}
			if err := json.Unmarshal(raw, &merged); err != nil {
				return mcp.MCPConfig{}, nil, nil, err
			}
			foundGlobal = true
			break
		}
	}
	if !foundGlobal {
		return mcp.MCPConfig{}, nil, nil, fmt.Errorf("global workspace mcp config not found")
	}
	if merged.Servers == nil {
		merged.Servers = map[string]mcp.MCPServerConfig{}
	}
	var added []string
	var deduplicated []string
	for _, workspace := range workspaces {
		if workspace.Name == globalWorkspace {
			continue
		}
		names := make([]string, 0, len(workspace.MCPConfig.Servers))
		for name := range workspace.MCPConfig.Servers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			server := workspace.MCPConfig.Servers[name]
			if existingName, ok := sameMCPConnection(merged.Servers, server); ok {
				deduplicated = append(deduplicated, workspace.Name+":"+name+"="+existingName)
				continue
			}
			candidate := name
			if existing, exists := merged.Servers[candidate]; exists {
				if reflect.DeepEqual(existing, server) {
					deduplicated = append(deduplicated, workspace.Name+":"+name+"="+candidate)
					continue
				}
				base := name + "-" + modelprovider.NormalizeID(workspace.Name)
				candidate = base
				for suffix := 2; ; suffix++ {
					if _, exists := merged.Servers[candidate]; !exists {
						break
					}
					candidate = fmt.Sprintf("%s-%d", base, suffix)
				}
			}
			merged.Servers[candidate] = server
			added = append(added, workspace.Name+":"+name+"->"+candidate)
		}
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return mcp.MCPConfig{}, nil, nil, err
	}
	validated, err := mcp.ParseMCPConfig(string(raw))
	if err != nil {
		return mcp.MCPConfig{}, nil, nil, err
	}
	sort.Strings(added)
	sort.Strings(deduplicated)
	return validated, added, deduplicated, nil
}

func sameMCPConnection(servers map[string]mcp.MCPServerConfig, candidate mcp.MCPServerConfig) (string, bool) {
	candidateURL := strings.TrimRight(strings.TrimSpace(candidate.URL), "/")
	for name, existing := range servers {
		if strings.TrimRight(strings.TrimSpace(existing.URL), "/") != candidateURL {
			continue
		}
		if strings.TrimSpace(existing.Type) != strings.TrimSpace(candidate.Type) {
			continue
		}
		if existing.Auth.Type == candidate.Auth.Type && existing.Auth.Token == candidate.Auth.Token && existing.Auth.TokenEnv == candidate.Auth.TokenEnv {
			return name, true
		}
	}
	return "", false
}

func insertLegacyProjects(writer sqlWriter, plan legacyWorkspaceMigrationPlan) ([]LegacyWorkspaceMigrationProject, error) {
	var projects []LegacyWorkspaceMigrationProject
	for _, workspace := range plan.Workspaces {
		if workspace.Name == plan.GlobalWorkspace {
			continue
		}
		projectID, err := normalizeProjectID(workspace.Name)
		if err != nil {
			return nil, fmt.Errorf("workspace %s cannot become a project: %w", workspace.Name, err)
		}
		projectName, err := normalizeProjectName(workspace.Name)
		if err != nil {
			return nil, fmt.Errorf("workspace %s cannot become a project: %w", workspace.Name, err)
		}
		prompt := strings.TrimSpace(workspace.Config.SystemPrompt)
		if _, err := writer.Exec(`INSERT INTO projects(id, name, prompt, created_at, updated_at) VALUES(?, ?, ?, ?, ?)`, projectID, projectName, prompt, workspace.CreatedAt, workspace.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, LegacyWorkspaceMigrationProject{ID: projectID, Name: projectName, SessionCount: workspace.Sessions})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

func copyNonLegacyMeta(source sqlQueryer, target sqlWriter) error {
	rows, err := source.Query(`SELECT key, value FROM meta ORDER BY key`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		if key == modelprovider.MetaKey || legacyMigrationMetaKeys[key] {
			continue
		}
		if err := setMetaValueWith(target, key, value); err != nil {
			return err
		}
	}
	return rows.Err()
}

func existingIgnoredLegacyTables(reader sqlQueryer) []string {
	var ignored []string
	for _, table := range []string{"lost_and_found", "schema_migrations", "workspaces", "workspace_kv"} {
		exists, err := sqliteTableExists(reader, table)
		if err == nil && exists {
			ignored = append(ignored, table)
		}
	}
	return ignored
}

func validateLegacyWorkspaceSchema(reader sqlQueryer) error {
	for _, table := range []string{"global_settings", "projects"} {
		exists, err := sqliteTableExists(reader, table)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("database is already using the current project schema: %s exists", table)
		}
	}
	for _, table := range legacyRequiredTables() {
		exists, err := sqliteTableExists(reader, table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("legacy workspace table is missing: %s", table)
		}
	}
	for _, table := range []string{"sessions", "session_messages", "session_message_parts", "session_message_events", "session_message_event_details", "mcp_runs", "mcp_confirmations", "chat_jobs", "scheduled_tasks", "scheduled_task_runs", "attachments", "tool_embeddings"} {
		hasWorkspace, err := sqliteColumnExists(reader, table, "workspace_id")
		if err != nil {
			return err
		}
		if !hasWorkspace {
			return fmt.Errorf("legacy workspace column is missing: %s.workspace_id", table)
		}
	}
	return nil
}

func validateLegacyGlobalKeys(reader sqlQueryer) error {
	for _, table := range []string{"sessions", "scheduled_tasks", "scheduled_task_runs", "tool_embeddings"} {
		var total, distinct int64
		expression := "id"
		if table == "tool_embeddings" {
			expression = `full_name || char(0) || embedding_model`
		}
		query := fmt.Sprintf(`SELECT COUNT(*), COUNT(DISTINCT %s) FROM %s`, expression, table)
		if err := reader.QueryRow(query).Scan(&total, &distinct); err != nil {
			return err
		}
		if total != distinct {
			return fmt.Errorf("dropping workspace scope would create duplicate keys in %s", table)
		}
	}
	return nil
}
