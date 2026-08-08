package legacyworkspace

import (
	"chatdock/internal/modelprovider"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"

	_ "github.com/mattn/go-sqlite3"
)

func TestMigrateLegacyWorkspacesPreservesDataAndBuildsProjects(t *testing.T) {
	sourcePath := createLegacyWorkspaceMigrationFixture(t)
	sourceHash, err := sha256File(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(t.TempDir(), "target")
	targetPath := filepath.Join(targetDir, "chatdock.sqlite")

	report, err := MigrateLegacyWorkspaces(LegacyWorkspaceMigrationOptions{
		SourcePath:      sourcePath,
		TargetPath:      targetPath,
		GlobalWorkspace: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.SourceSHA256 != sourceHash || report.TargetSHA256 == "" {
		t.Fatalf("unexpected migration hashes: %#v", report)
	}
	if len(report.Projects) != 1 || report.Projects[0].ID != "MCP" || report.Projects[0].SessionCount != 1 {
		t.Fatalf("projects = %#v", report.Projects)
	}
	if len(report.LegacyProviders) != 1 || report.LegacyProviders[0].ProviderID != "legacy-mcp" {
		t.Fatalf("legacy providers = %#v", report.LegacyProviders)
	}
	if len(report.AddedMCPServers) != 1 || report.AddedMCPServers[0] != "MCP:agentdock->agentdock" {
		t.Fatalf("added MCP servers = %#v", report.AddedMCPServers)
	}
	if report.RemappedProviderAliases["provider_default->default"] != 1 {
		t.Fatalf("provider aliases = %#v", report.RemappedProviderAliases)
	}
	if len(report.UnresolvedProviderIDs) != 0 {
		t.Fatalf("unresolved providers = %#v", report.UnresolvedProviderIDs)
	}

	sourceHashAfter, err := sha256File(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if sourceHashAfter != sourceHash {
		t.Fatalf("source database changed: before=%s after=%s", sourceHash, sourceHashAfter)
	}

	currentStore, err := storepkg.NewStore(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	defer currentStore.Close()
	projects, err := currentStore.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if projects.SessionCount != 3 || projects.PlainSessionCount != 2 || len(projects.Projects) != 1 || projects.Projects[0].SessionCount != 1 {
		t.Fatalf("project counts = %#v", projects)
	}
	if projects.Projects[0].Prompt != "MCP 项目提示词" {
		t.Fatalf("project prompt = %q", projects.Projects[0].Prompt)
	}
	cfg, err := currentStore.ModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderID != "default" || cfg.Model != "global-model" || cfg.SystemPrompt != "全局系统提示词" {
		t.Fatalf("global config = %#v", cfg)
	}
	mcpRaw, err := currentStore.GetMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	mcpConfig, err := mcp.ParseMCPConfig(mcpRaw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mcpConfig.Servers["DockMini"]; !ok {
		t.Fatalf("global MCP config lost DockMini: %#v", mcpConfig.Servers)
	}
	if _, ok := mcpConfig.Servers["agentdock"]; !ok {
		t.Fatalf("global MCP config lost agentdock: %#v", mcpConfig.Servers)
	}
	providers, err := currentStore.ListModelProviders()
	if err != nil {
		t.Fatal(err)
	}
	if !hasProvider(providers, "legacy-mcp", "https://legacy-mcp.example/v1", "mcp-model") {
		t.Fatalf("legacy MCP provider missing: %#v", providers)
	}

	validationDB := openSQLiteForTest(t, targetPath)
	defer validationDB.Close()

	assertMigratedSession(t, validationDB, "s-default", "", "default", "global-model")
	assertMigratedSession(t, validationDB, "s-inherit", "", "", "")
	assertMigratedSession(t, validationDB, "s-mcp", "MCP", "legacy-mcp", "mcp-model")
	for table, want := range map[string]int64{
		"sessions":                      3,
		"session_messages":              1,
		"session_message_parts":         1,
		"session_message_events":        1,
		"session_message_event_details": 1,
		"mcp_runs":                      1,
		"mcp_run_events":                1,
		"mcp_confirmations":             1,
		"chat_jobs":                     1,
		"chat_job_events":               1,
		"scheduled_tasks":               1,
		"scheduled_task_runs":           1,
		"attachments":                   1,
		"attachment_blobs":              1,
		"tool_embeddings":               1,
	} {
		assertTableCount(t, validationDB, table, want)
	}
	for _, table := range []string{"workspaces", "workspace_kv", "schema_migrations"} {
		if exists, err := sqliteTableExists(validationDB, table); err != nil {
			t.Fatal(err)
		} else if exists {
			t.Fatalf("legacy table remains: %s", table)
		}
	}
}

func TestMigrateLegacyWorkspacesRejectsUnsafeInputs(t *testing.T) {
	t.Run("existing target", func(t *testing.T) {
		sourcePath := createLegacyWorkspaceMigrationFixture(t)
		targetPath := filepath.Join(t.TempDir(), "target.sqlite")
		if err := os.WriteFile(targetPath, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(targetPath)
		_, err := MigrateLegacyWorkspaces(LegacyWorkspaceMigrationOptions{SourcePath: sourcePath, TargetPath: targetPath})
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("error = %v", err)
		}
		after, _ := os.ReadFile(targetPath)
		if string(after) != string(before) {
			t.Fatal("existing target was modified")
		}
	})

	t.Run("current schema", func(t *testing.T) {
		sourceDir := t.TempDir()
		current, err := storepkg.NewStore(sourceDir)
		if err != nil {
			t.Fatal(err)
		}
		if err := current.Close(); err != nil {
			t.Fatal(err)
		}
		targetPath := filepath.Join(t.TempDir(), "target.sqlite")
		_, err = MigrateLegacyWorkspaces(LegacyWorkspaceMigrationOptions{SourcePath: filepath.Join(sourceDir, "chatdock.sqlite"), TargetPath: targetPath})
		if err == nil || !strings.Contains(err.Error(), "already using the current project schema") {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
			t.Fatalf("target should not exist: %v", statErr)
		}
	})

	t.Run("SQLite sidecar", func(t *testing.T) {
		sourcePath := createLegacyWorkspaceMigrationFixture(t)
		if err := os.WriteFile(sourcePath+"-wal", []byte("not a standalone snapshot"), 0o600); err != nil {
			t.Fatal(err)
		}
		targetPath := filepath.Join(t.TempDir(), "target.sqlite")
		_, err := MigrateLegacyWorkspaces(LegacyWorkspaceMigrationOptions{SourcePath: sourcePath, TargetPath: targetPath})
		if err == nil || !strings.Contains(err.Error(), "standalone SQLite backup") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("global key collision", func(t *testing.T) {
		sourcePath := createLegacyWorkspaceMigrationFixture(t)
		db := openSQLiteForTest(t, sourcePath)
		if _, err := db.Exec(`INSERT INTO sessions(workspace_id, id, json, created_at, updated_at, title, pinned, provider_id, model) VALUES('MCP', 's-default', '{}', ?, ?, 'duplicate', 0, '', '')`, migrationTestTime(), migrationTestTime()); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		targetPath := filepath.Join(t.TempDir(), "target.sqlite")
		_, err := MigrateLegacyWorkspaces(LegacyWorkspaceMigrationOptions{SourcePath: sourcePath, TargetPath: targetPath})
		if err == nil || !strings.Contains(err.Error(), "duplicate keys in sessions") {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Stat(targetPath); !os.IsNotExist(statErr) {
			t.Fatalf("target should not exist: %v", statErr)
		}
	})
}

func createLegacyWorkspaceMigrationFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db := openSQLiteForTest(t, path)
	defer db.Close()
	for _, statement := range legacyMigrationFixtureSchema() {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create legacy schema: %v\n%s", err, statement)
		}
	}
	now := migrationTestTime()
	if _, err := db.Exec(`INSERT INTO workspaces(name, created_at, updated_at) VALUES('default', ?, ?), ('MCP', ?, ?)`, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	globalConfig := model.ModelConfig{ProviderID: "default", BaseURL: "https://global.example/v1", APIKey: "global-key", Model: "global-model", Models: []string{"global-model"}, SystemPrompt: "全局系统提示词", ContextMode: model.ContextModeAuto, MaxContextMessages: 12, Temperature: 0.7}
	projectConfig := model.ModelConfig{ProviderID: "mcp", BaseURL: "https://legacy-mcp.example/v1", Model: "mcp-model", Models: []string{"mcp-model"}, SystemPrompt: "MCP 项目提示词", ContextMode: model.ContextModeAuto, MaxContextMessages: 12, Temperature: 0.3}
	globalMCP := `{"builtin_tools":{"tool_exposure":"direct"},"servers":{"DockMini":{"url":"http://agentdock.test/mcp","auth":{"type":"bearer","token":"global-token"}}}}`
	projectMCP := `{"servers":{"agentdock":{"type":"streamable-http","url":"http://agentdock.test/mcp","auth":{"type":"bearer","token":"project-token"}}}}`
	insertWorkspaceJSON(t, db, "default", "config", globalConfig)
	insertWorkspaceJSON(t, db, "MCP", "config", projectConfig)
	insertWorkspaceRaw(t, db, "default", "mcp", globalMCP)
	insertWorkspaceRaw(t, db, "MCP", "mcp", projectMCP)

	enabled := true
	providers := []modelprovider.Record{
		modelprovider.NormalizeRecord(modelprovider.Record{ID: "default", Name: "Global", Type: "openai-compatible", BaseURL: globalConfig.BaseURL, APIKeys: modelprovider.UpsertAPIKey(nil, "", globalConfig.APIKey, time.Now()), DefaultModel: globalConfig.Model, Models: globalConfig.Models, TimeoutMS: 120000, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}),
		modelprovider.NormalizeRecord(modelprovider.Record{ID: "mcp", Name: "Existing MCP", Type: "openai-compatible", BaseURL: "https://other-mcp.example/v1", APIKeys: modelprovider.UpsertAPIKey(nil, "", "other-key", time.Now()), DefaultModel: "other-model", Models: []string{"other-model"}, TimeoutMS: 120000, Enabled: enabled, CreatedAt: time.Now(), UpdatedAt: time.Now()}),
	}
	if err := modelprovider.SaveRecords(db, providers); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO meta(key, value) VALUES('json_migrated', '1'), ('custom_meta', 'keep')`); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]any{
		{"default", "s-default", `{"id":"s-default"}`, "Default", "provider_default", "global-model"},
		{"default", "s-inherit", `{"id":"s-inherit"}`, "Inherit", "", ""},
		{"MCP", "s-mcp", `{"id":"s-mcp"}`, "Project", "", ""},
	} {
		if _, err := db.Exec(`INSERT INTO sessions(workspace_id, id, json, created_at, updated_at, title, pinned, provider_id, model) VALUES(?, ?, ?, ?, ?, ?, 0, ?, ?)`, args[0], args[1], args[2], now, now, args[3], args[4], args[5]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO session_messages(workspace_id, session_id, message_index, id, role, content, reasoning, attachments_json, created_at, error_json) VALUES('default', 's-default', 0, 'msg-1', 'user', 'hello', '', '[]', ?, '')`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session_message_parts(workspace_id, session_id, message_index, part_index, kind, text, call_key, event_id) VALUES('default', 's-default', 0, 0, 'text', 'hello', '', '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session_message_events(workspace_id, session_id, message_index, event_index, id, kind, phase, call_key, text, meta) VALUES('default', 's-default', 0, 0, 'event-1', 'tool', 'done', '', 'ok', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session_message_event_details(workspace_id, session_id, event_id, details_json, details_bytes, updated_at) VALUES('default', 's-default', 'event-1', '{"ok":true}', 11, ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mcp_runs(workspace_id, id, session_id, title, status, summary, error, started_at, finished_at, duration_ms, event_count, updated_at) VALUES('default', 'run-1', 's-default', 'run', 'done', 'ok', '', ?, ?, 1, 1, ?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mcp_run_events(id, run_id, seq, kind, status, server, tool, action, summary, arguments_json, result_json, error, duration_ms, started_at, finished_at, created_at) VALUES('run-event-1', 'run-1', 0, 'tool', 'done', 'DockMini', 'test', '', 'ok', '{}', '{}', '', 1, ?, ?, ?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mcp_confirmations(workspace_id, id, session_id, tool, arguments_json, status, requested_at, resolved_at, message) VALUES('default', 'confirm-1', 's-default', 'test', '{}', 'approved', ?, ?, 'ok')`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_jobs(workspace_id, id, session_id, status, answer, reasoning, error, started_at, finished_at, updated_at, request_id) VALUES('default', 'job-1', 's-default', 'done', 'answer', '', '', ?, ?, ?, 'request-1')`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_job_events(job_id, seq, event, data_json, created_at) VALUES('job-1', 0, 'done', '{}', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO scheduled_tasks(workspace_id, id, title, task_prompt, enabled, running, schedule_type, run_at, cron_expressions, timezone, interval_minutes, context_mode, next_run_at, last_run_at, last_status, last_error, session_id, created_at, updated_at) VALUES('default', 'task-1', 'task', 'prompt', 1, 0, 'interval', '', '[]', '', 60, 'stateless', '', '', '', '', '', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO scheduled_task_runs(workspace_id, id, task_id, task_title, task_prompt, output, status, error, manual, session_id, started_at, finished_at, duration_ms) VALUES('default', 'task-run-1', 'task-1', 'task', 'prompt', 'ok', 'done', '', 1, '', ?, ?, 1)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO attachment_blobs(sha256, storage_path, size, mime_type, ref_count, created_at) VALUES('sha-1', '/data/attachments/sha-1', 3, 'text/plain', 1, ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO attachments(workspace_id, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at) VALUES('default', 'attachment-1', 's-default', 'msg-1', 'a.txt', 'text/plain', 3, '/data/attachments/sha-1', 'sha-1', 'abc', 'ready', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tool_embeddings(workspace_id, full_name, source_hash, embedding_model, embedding_json, indexed_at, embedding_blob) VALUES('default', 'tool-1', 'source', 'embed-model', '[1]', ?, x'01')`, now); err != nil {
		t.Fatal(err)
	}
	return path
}

func legacyMigrationFixtureSchema() []string {
	return []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE workspaces (name TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE workspace_kv (workspace_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, key), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE sessions (workspace_id TEXT NOT NULL, id TEXT NOT NULL, json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', pinned INTEGER NOT NULL DEFAULT 0, provider_id TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, id), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE session_messages (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, message_index INTEGER NOT NULL, id TEXT NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL, reasoning TEXT NOT NULL DEFAULT '', attachments_json TEXT NOT NULL DEFAULT '[]', created_at TEXT NOT NULL, error_json TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, session_id, message_index), FOREIGN KEY(workspace_id, session_id) REFERENCES sessions(workspace_id, id) ON DELETE CASCADE)`,
		`CREATE TABLE session_message_parts (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, message_index INTEGER NOT NULL, part_index INTEGER NOT NULL, kind TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', event_id TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, session_id, message_index, part_index), FOREIGN KEY(workspace_id, session_id, message_index) REFERENCES session_messages(workspace_id, session_id, message_index) ON DELETE CASCADE)`,
		`CREATE TABLE session_message_events (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, message_index INTEGER NOT NULL, event_index INTEGER NOT NULL, id TEXT NOT NULL, kind TEXT NOT NULL, phase TEXT NOT NULL DEFAULT '', call_key TEXT NOT NULL DEFAULT '', text TEXT NOT NULL DEFAULT '', meta TEXT NOT NULL DEFAULT '', PRIMARY KEY(workspace_id, session_id, message_index, event_index), FOREIGN KEY(workspace_id, session_id, message_index) REFERENCES session_messages(workspace_id, session_id, message_index) ON DELETE CASCADE)`,
		`CREATE TABLE session_message_event_details (workspace_id TEXT NOT NULL, session_id TEXT NOT NULL, event_id TEXT NOT NULL, details_json TEXT NOT NULL, details_bytes INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, PRIMARY KEY(workspace_id, session_id, event_id), FOREIGN KEY(workspace_id, session_id) REFERENCES sessions(workspace_id, id) ON DELETE CASCADE)`,
		`CREATE TABLE mcp_runs (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, title TEXT NOT NULL, status TEXT NOT NULL, summary TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, event_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE mcp_run_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, server TEXT NOT NULL, tool TEXT NOT NULL, action TEXT NOT NULL, summary TEXT NOT NULL, arguments_json TEXT NOT NULL, result_json TEXT NOT NULL, error TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(run_id) REFERENCES mcp_runs(id) ON DELETE CASCADE)`,
		`CREATE TABLE mcp_confirmations (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL, arguments_json TEXT NOT NULL DEFAULT '{}', status TEXT NOT NULL, requested_at TEXT NOT NULL, resolved_at TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '', FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE chat_jobs (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, status TEXT NOT NULL, answer TEXT NOT NULL, reasoning TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, updated_at TEXT NOT NULL, request_id TEXT NOT NULL DEFAULT '', FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE chat_job_events (job_id TEXT NOT NULL, seq INTEGER NOT NULL, event TEXT NOT NULL, data_json TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY(job_id, seq), FOREIGN KEY(job_id) REFERENCES chat_jobs(id) ON DELETE CASCADE)`,
		`CREATE TABLE scheduled_tasks (workspace_id TEXT NOT NULL, id TEXT NOT NULL, title TEXT NOT NULL, task_prompt TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, running INTEGER NOT NULL DEFAULT 0, schedule_type TEXT NOT NULL, run_at TEXT NOT NULL DEFAULT '', cron_expressions TEXT NOT NULL DEFAULT '[]', timezone TEXT NOT NULL DEFAULT '', interval_minutes INTEGER NOT NULL DEFAULT 0, context_mode TEXT NOT NULL DEFAULT 'stateless', next_run_at TEXT NOT NULL DEFAULT '', last_run_at TEXT NOT NULL DEFAULT '', last_status TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '', session_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(workspace_id, id), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE scheduled_task_runs (workspace_id TEXT NOT NULL, id TEXT NOT NULL, task_id TEXT NOT NULL, task_title TEXT NOT NULL, task_prompt TEXT NOT NULL, output TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', manual INTEGER NOT NULL DEFAULT 0, session_id TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL, finished_at TEXT NOT NULL DEFAULT '', duration_ms INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(workspace_id, id), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE attachments (workspace_id TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, message_id TEXT NOT NULL, filename TEXT NOT NULL, mime_type TEXT NOT NULL, size INTEGER NOT NULL, storage_path TEXT NOT NULL, sha256 TEXT NOT NULL, text_content TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE attachment_blobs (sha256 TEXT PRIMARY KEY, storage_path TEXT NOT NULL, size INTEGER NOT NULL, mime_type TEXT NOT NULL, ref_count INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE TABLE tool_embeddings (workspace_id TEXT NOT NULL, full_name TEXT NOT NULL, source_hash TEXT NOT NULL, embedding_model TEXT NOT NULL, embedding_json TEXT NOT NULL, indexed_at TEXT NOT NULL, embedding_blob BLOB NOT NULL DEFAULT x'', PRIMARY KEY(workspace_id, full_name, embedding_model), FOREIGN KEY(workspace_id) REFERENCES workspaces(name) ON DELETE CASCADE)`,
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`,
	}
}

func insertWorkspaceJSON(t *testing.T, db *sql.DB, workspace string, key string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	insertWorkspaceRaw(t, db, workspace, key, string(raw))
}

func insertWorkspaceRaw(t *testing.T, db *sql.DB, workspace string, key string, value string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO workspace_kv(workspace_id, key, value, updated_at) VALUES(?, ?, ?, ?)`, workspace, key, value, migrationTestTime()); err != nil {
		t.Fatal(err)
	}
}

func openSQLiteForTest(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func migrationTestTime() string {
	return "2026-07-23T12:00:00Z"
}

func hasProvider(providers []modelprovider.Provider, id string, baseURL string, modelName string) bool {
	for _, provider := range providers {
		if provider.ID == id && provider.BaseURL == baseURL && provider.DefaultModel == modelName {
			return true
		}
	}
	return false
}

func assertMigratedSession(t *testing.T, db *sql.DB, id string, projectID string, providerID string, modelName string) {
	t.Helper()
	var gotProject sql.NullString
	var gotProvider, gotModel string
	if err := db.QueryRow(`SELECT project_id, provider_id, model FROM sessions WHERE id = ?`, id).Scan(&gotProject, &gotProvider, &gotModel); err != nil {
		t.Fatal(err)
	}
	if gotProject.String != projectID || gotProvider != providerID || gotModel != modelName {
		t.Fatalf("session %s = project %q provider %q model %q", id, gotProject.String, gotProvider, gotModel)
	}
}

func assertTableCount(t *testing.T, db *sql.DB, table string, want int64) {
	t.Helper()
	var got int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("table %s count = %d, want %d", table, got, want)
	}
}
