package legacyworkspace

import (
	"chatdock/internal/modelprovider"
	"database/sql"
	"fmt"
	"strings"

	"chatdock/internal/mcp"
)

type legacySessionCopyReport struct {
	Count               int64
	RemappedAliases     map[string]int64
	UnresolvedProviders map[string]int64
}

type legacyTableCopySpec struct {
	Name        string
	SourceQuery string
	InsertSQL   string
}

func legacyRequiredTables() []string {
	return []string{
		"meta",
		"workspaces",
		"workspace_kv",
		"sessions",
		"session_messages",
		"session_message_parts",
		"session_message_events",
		"session_message_event_details",
		"mcp_runs",
		"mcp_run_events",
		"mcp_confirmations",
		"chat_jobs",
		"chat_job_events",
		"scheduled_tasks",
		"scheduled_task_runs",
		"attachments",
		"attachment_blobs",
		"tool_embeddings",
	}
}

func legacyMigrationTableSpecs() []legacyTableCopySpec {
	return []legacyTableCopySpec{
		{
			Name:        "session_messages",
			SourceQuery: `SELECT session_id, message_index, id, role, content, reasoning, error_json, attachments_json, created_at FROM session_messages ORDER BY workspace_id, session_id, message_index`,
			InsertSQL:   `INSERT INTO session_messages(session_id, message_index, id, role, content, reasoning, error_json, attachments_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "session_message_parts",
			SourceQuery: `SELECT session_id, message_index, part_index, kind, text, call_key, event_id FROM session_message_parts ORDER BY workspace_id, session_id, message_index, part_index`,
			InsertSQL:   `INSERT INTO session_message_parts(session_id, message_index, part_index, kind, text, call_key, event_id) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "session_message_events",
			SourceQuery: `SELECT session_id, message_index, event_index, id, kind, phase, call_key, text, meta FROM session_message_events ORDER BY workspace_id, session_id, message_index, event_index`,
			InsertSQL:   `INSERT INTO session_message_events(session_id, message_index, event_index, id, kind, phase, call_key, text, meta) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "session_message_event_details",
			SourceQuery: `SELECT session_id, event_id, details_json, details_bytes, updated_at FROM session_message_event_details ORDER BY workspace_id, session_id, event_id`,
			InsertSQL:   `INSERT INTO session_message_event_details(session_id, event_id, details_json, details_bytes, updated_at) VALUES(?, ?, ?, ?, ?)`,
		},
		{
			Name:        "mcp_runs",
			SourceQuery: `SELECT id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at FROM mcp_runs ORDER BY id`,
			InsertSQL:   `INSERT INTO mcp_runs(id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "mcp_run_events",
			SourceQuery: `SELECT id, run_id, seq, kind, status, server, tool, action, summary, arguments_json, result_json, error, duration_ms, started_at, finished_at, created_at FROM mcp_run_events ORDER BY run_id, seq`,
			InsertSQL:   `INSERT INTO mcp_run_events(id, run_id, seq, kind, status, server, tool, action, summary, arguments_json, result_json, error, duration_ms, started_at, finished_at, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "mcp_confirmations",
			SourceQuery: `SELECT id, session_id, tool, arguments_json, status, requested_at, resolved_at, message FROM mcp_confirmations ORDER BY id`,
			InsertSQL:   `INSERT INTO mcp_confirmations(id, session_id, tool, arguments_json, status, requested_at, resolved_at, message) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "chat_jobs",
			SourceQuery: `SELECT id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at FROM chat_jobs ORDER BY id`,
			InsertSQL:   `INSERT INTO chat_jobs(id, session_id, request_id, status, answer, reasoning, error, started_at, finished_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "chat_job_events",
			SourceQuery: `SELECT job_id, seq, event, data_json, created_at FROM chat_job_events ORDER BY job_id, seq`,
			InsertSQL:   `INSERT INTO chat_job_events(job_id, seq, event, data_json, created_at) VALUES(?, ?, ?, ?, ?)`,
		},
		{
			Name: "scheduled_tasks",
			SourceQuery: `SELECT id, title, task_prompt, enabled, running, schedule_type, run_at, cron_expressions, timezone, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at
				FROM scheduled_tasks ORDER BY workspace_id, id`,
			InsertSQL: `INSERT INTO scheduled_tasks(id, title, task_prompt, enabled, running, schedule_type, run_at, cron_expressions, timezone, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at)
				VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name: "scheduled_task_runs",
			SourceQuery: `SELECT id, task_id, task_title, task_prompt, output, status, error, manual, session_id, started_at, finished_at, duration_ms
				FROM scheduled_task_runs ORDER BY workspace_id, id`,
			InsertSQL: `INSERT INTO scheduled_task_runs(id, task_id, task_title, task_prompt, output, status, error, manual, session_id, started_at, finished_at, duration_ms)
				VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "attachment_blobs",
			SourceQuery: `SELECT sha256, storage_path, size, mime_type, ref_count, created_at FROM attachment_blobs ORDER BY sha256`,
			InsertSQL:   `INSERT INTO attachment_blobs(sha256, storage_path, size, mime_type, ref_count, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "attachments",
			SourceQuery: `SELECT id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at FROM attachments ORDER BY id`,
			InsertSQL:   `INSERT INTO attachments(id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		},
		{
			Name:        "tool_embeddings",
			SourceQuery: `SELECT full_name, source_hash, embedding_model, embedding_json, embedding_blob, indexed_at FROM tool_embeddings ORDER BY full_name, embedding_model`,
			InsertSQL:   `INSERT INTO tool_embeddings(full_name, source_hash, embedding_model, embedding_json, embedding_blob, indexed_at) VALUES(?, ?, ?, ?, ?, ?)`,
		},
	}
}

func legacyMigrationTableCounts(reader sqlQueryer) (map[string]int64, error) {
	counts := make(map[string]int64, len(legacyMigrationTableSpecs())+1)
	var sessions int64
	if err := reader.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		return nil, err
	}
	counts["sessions"] = sessions
	for _, spec := range legacyMigrationTableSpecs() {
		var count int64
		if err := reader.QueryRow(`SELECT COUNT(*) FROM ` + spec.Name).Scan(&count); err != nil {
			return nil, err
		}
		counts[spec.Name] = count
	}
	return counts, nil
}

func copyLegacySessions(source sqlQueryer, target sqlWriter, plan legacyWorkspaceMigrationPlan, workspaceProviders map[string]LegacyWorkspaceMigrationProvider, providers []modelprovider.Record) (legacySessionCopyReport, error) {
	rows, err := source.Query(`SELECT workspace_id, id, title, pinned, provider_id, model, created_at, updated_at FROM sessions ORDER BY workspace_id, id`)
	if err != nil {
		return legacySessionCopyReport{}, err
	}
	defer rows.Close()
	statement, err := prepareSQLWriter(target, `INSERT INTO sessions(id, project_id, title, pinned, provider_id, model, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return legacySessionCopyReport{}, err
	}
	defer statement.Close()

	providerByID := make(map[string]modelprovider.Record, len(providers))
	for _, provider := range providers {
		providerByID[provider.ID] = provider
	}
	globalProvider := providerByID[modelprovider.NormalizeID(plan.GlobalConfig.ProviderID)]
	report := legacySessionCopyReport{
		RemappedAliases:     map[string]int64{},
		UnresolvedProviders: map[string]int64{},
	}
	for rows.Next() {
		var workspaceID, id, title, providerID, modelName, createdAt, updatedAt string
		var pinned int
		if err := rows.Scan(&workspaceID, &id, &title, &pinned, &providerID, &modelName, &createdAt, &updatedAt); err != nil {
			return legacySessionCopyReport{}, err
		}
		providerID = strings.TrimSpace(providerID)
		modelName = strings.TrimSpace(modelName)
		var projectID any
		if workspaceID != plan.GlobalWorkspace {
			projectID = workspaceID
			fallback, ok := workspaceProviders[workspaceID]
			if !ok && (providerID == "" || modelName == "") {
				return legacySessionCopyReport{}, fmt.Errorf("workspace %s session %s has no migration model", workspaceID, id)
			}
			if providerID == "" {
				providerID = fallback.ProviderID
			}
			if modelName == "" {
				modelName = fallback.Model
			}
		}
		if providerID == "provider_default" {
			if _, exists := providerByID[providerID]; !exists && providerSupportsModel(globalProvider, modelName) {
				providerID = globalProvider.ID
				report.RemappedAliases["provider_default->"+globalProvider.ID]++
			}
		}
		if providerID != "" {
			if _, exists := providerByID[providerID]; !exists {
				report.UnresolvedProviders[providerID]++
			}
		}
		if _, err := statement.Exec(id, projectID, title, pinned, providerID, modelName, createdAt, updatedAt); err != nil {
			return legacySessionCopyReport{}, err
		}
		report.Count++
	}
	if err := rows.Err(); err != nil {
		return legacySessionCopyReport{}, err
	}
	if len(report.RemappedAliases) == 0 {
		report.RemappedAliases = nil
	}
	if len(report.UnresolvedProviders) == 0 {
		report.UnresolvedProviders = nil
	}
	return report, nil
}

func providerSupportsModel(provider modelprovider.Record, modelName string) bool {
	if provider.ID == "" {
		return false
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == strings.TrimSpace(provider.DefaultModel) {
		return true
	}
	for _, candidate := range provider.Models {
		if strings.TrimSpace(candidate) == modelName {
			return true
		}
	}
	return false
}

func copyLegacyWorkspaceTables(source sqlQueryer, target sqlWriter) (map[string]int64, error) {
	counts := make(map[string]int64, len(legacyMigrationTableSpecs()))
	for _, spec := range legacyMigrationTableSpecs() {
		count, err := copyLegacyTable(source, target, spec)
		if err != nil {
			return nil, fmt.Errorf("copy %s: %w", spec.Name, err)
		}
		counts[spec.Name] = count
	}
	return counts, nil
}

func copyLegacyTable(source sqlQueryer, target sqlWriter, spec legacyTableCopySpec) (int64, error) {
	rows, err := source.Query(spec.SourceQuery)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	statement, err := prepareSQLWriter(target, spec.InsertSQL)
	if err != nil {
		return 0, err
	}
	defer statement.Close()

	var count int64
	for rows.Next() {
		// 这些表只去掉 workspace_id，剩余列按原值逐列复制。这里使用数据库边界的
		// 动态值，避免为十多个仅列数不同的历史表重复维护扫描结构体。
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return 0, err
		}
		if _, err := statement.Exec(values...); err != nil {
			return 0, err
		}
		count++
	}
	return count, rows.Err()
}

func prepareSQLWriter(writer sqlWriter, query string) (*sql.Stmt, error) {
	type preparer interface {
		Prepare(string) (*sql.Stmt, error)
	}
	preparable, ok := writer.(preparer)
	if !ok {
		return nil, fmt.Errorf("sql writer does not support prepared statements")
	}
	return preparable.Prepare(query)
}

func validateMigratedWorkspaceDatabase(db *sql.DB, plan legacyWorkspaceMigrationPlan, report LegacyWorkspaceMigrationReport) error {
	if err := requireSQLiteCheck(db, "quick_check"); err != nil {
		return err
	}
	if err := requireNoForeignKeyIssues(db); err != nil {
		return err
	}
	if err := rejectLegacyWorkspaceSchema(db); err != nil {
		return err
	}
	for _, table := range []string{"schema_migrations", "workspaces", "workspace_kv"} {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("legacy table remains after migration: %s", table)
		}
	}
	if hasJSON, err := sqliteColumnExists(db, "sessions", "json"); err != nil {
		return err
	} else if hasJSON {
		return fmt.Errorf("legacy sessions.json column remains after migration")
	}
	for _, table := range []string{"sessions", "session_messages", "session_message_parts", "session_message_events", "session_message_event_details", "mcp_runs", "mcp_confirmations", "chat_jobs", "scheduled_tasks", "scheduled_task_runs", "attachments", "tool_embeddings"} {
		hasWorkspace, err := sqliteColumnExists(db, table, "workspace_id")
		if err != nil {
			return err
		}
		if hasWorkspace {
			return fmt.Errorf("legacy workspace column remains after migration: %s.workspace_id", table)
		}
	}
	for table, sourceCount := range plan.TableCounts {
		var targetCount int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&targetCount); err != nil {
			return err
		}
		if targetCount != sourceCount {
			return fmt.Errorf("table %s row count changed: source=%d target=%d", table, sourceCount, targetCount)
		}
		if report.TableCounts[table] != sourceCount {
			return fmt.Errorf("table %s migration report count changed: source=%d report=%d", table, sourceCount, report.TableCounts[table])
		}
	}
	var projectCount int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projectCount); err != nil {
		return err
	}
	if projectCount != int64(len(plan.Workspaces)-1) {
		return fmt.Errorf("project count changed: got=%d want=%d", projectCount, len(plan.Workspaces)-1)
	}
	for _, workspace := range plan.Workspaces {
		if workspace.Name == plan.GlobalWorkspace {
			var plainCount int64
			if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE project_id IS NULL`).Scan(&plainCount); err != nil {
				return err
			}
			if plainCount != workspace.Sessions {
				return fmt.Errorf("plain session count changed: got=%d want=%d", plainCount, workspace.Sessions)
			}
			continue
		}
		var projectSessions int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE project_id = ?`, workspace.Name).Scan(&projectSessions); err != nil {
			return err
		}
		if projectSessions != workspace.Sessions {
			return fmt.Errorf("project %s session count changed: got=%d want=%d", workspace.Name, projectSessions, workspace.Sessions)
		}
	}
	if _, err := modelConfigWith(db); err != nil {
		return fmt.Errorf("load global model config: %w", err)
	}
	mcpRaw, ok, err := getGlobalRawWith(db, "mcp")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("global mcp config is missing")
	}
	if _, err := mcpParseForMigration(mcpRaw); err != nil {
		return err
	}
	if _, err := modelprovider.LoadRecords(db); err != nil {
		return fmt.Errorf("load global model providers: %w", err)
	}
	return nil
}

func mcpParseForMigration(content string) (bool, error) {
	if _, err := mcp.ParseMCPConfig(content); err != nil {
		return false, fmt.Errorf("load global mcp config: %w", err)
	}
	return true, nil
}

func requireSQLiteCheck(reader sqlQueryer, pragma string) error {
	rows, err := reader.Query(`PRAGMA ` + pragma)
	if err != nil {
		return err
	}
	defer rows.Close()
	var results []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(results) != 1 || strings.TrimSpace(strings.ToLower(results[0])) != "ok" {
		return fmt.Errorf("sqlite %s failed: %s", pragma, strings.Join(results, "; "))
	}
	return nil
}

func requireNoForeignKeyIssues(reader sqlQueryer) error {
	rows, err := reader.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("sqlite foreign_key_check reported violations")
	}
	return rows.Err()
}
