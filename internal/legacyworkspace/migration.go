package legacyworkspace

import (
	"chatdock/internal/modelprovider"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

const legacyWorkspaceMigrationMetaKey = "workspace_project_migration_v1"

var legacyMigrationMetaKeys = map[string]bool{
	"attachment_blobs_migrated":     true,
	"json_migrated":                 true,
	"scheduled_tables_migrated":     true,
	"session_tables_migrated":       true,
	"tool_embedding_blobs_migrated": true,
	legacyWorkspaceMigrationMetaKey: true,
}

type LegacyWorkspaceMigrationOptions struct {
	SourcePath      string
	TargetPath      string
	GlobalWorkspace string
}

type LegacyWorkspaceMigrationProject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SessionCount int64  `json:"session_count"`
}

type LegacyWorkspaceMigrationProvider struct {
	Workspace  string `json:"workspace"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

type LegacyWorkspaceMigrationReport struct {
	GlobalWorkspace         string                             `json:"global_workspace"`
	SourceSHA256            string                             `json:"source_sha256"`
	TargetSHA256            string                             `json:"target_sha256"`
	Projects                []LegacyWorkspaceMigrationProject  `json:"projects"`
	LegacyProviders         []LegacyWorkspaceMigrationProvider `json:"legacy_providers,omitempty"`
	AddedMCPServers         []string                           `json:"added_mcp_servers,omitempty"`
	DeduplicatedMCPServers  []string                           `json:"deduplicated_mcp_servers,omitempty"`
	RemappedProviderAliases map[string]int64                   `json:"remapped_provider_aliases,omitempty"`
	UnresolvedProviderIDs   map[string]int64                   `json:"unresolved_provider_ids,omitempty"`
	TableCounts             map[string]int64                   `json:"table_counts"`
	IgnoredLegacyTables     []string                           `json:"ignored_legacy_tables,omitempty"`
}

type legacyWorkspaceRecord struct {
	Name      string
	CreatedAt string
	UpdatedAt string
	Config    model.ModelConfig
	ConfigRaw string
	MCPConfig mcp.MCPConfig
	MCPRaw    string
	Sessions  int64
}

type legacyWorkspaceMigrationPlan struct {
	GlobalWorkspace string
	GlobalConfig    model.ModelConfig
	GlobalMCP       mcp.MCPConfig
	Workspaces      []legacyWorkspaceRecord
	Providers       []modelprovider.Record
	TableCounts     map[string]int64
	SourceSHA256    string
}

// MigrateLegacyWorkspaces 把一个已经脱离 WAL/SHM 的旧工作空间数据库转换为全局配置 + 项目结构。
// 源库始终以只读方式打开；所有写入先落到目标目录中的临时数据库，完整验证后才原子改名。
func MigrateLegacyWorkspaces(options LegacyWorkspaceMigrationOptions) (LegacyWorkspaceMigrationReport, error) {
	options.SourcePath = strings.TrimSpace(options.SourcePath)
	options.TargetPath = strings.TrimSpace(options.TargetPath)
	options.GlobalWorkspace = strings.TrimSpace(options.GlobalWorkspace)
	if options.GlobalWorkspace == "" {
		options.GlobalWorkspace = "default"
	}
	if options.SourcePath == "" || options.TargetPath == "" {
		return LegacyWorkspaceMigrationReport{}, fmt.Errorf("source and target database paths are required")
	}

	sourcePath, err := filepath.Abs(options.SourcePath)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	targetPath, err := filepath.Abs(options.TargetPath)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if sourcePath == targetPath {
		return LegacyWorkspaceMigrationReport{}, fmt.Errorf("source and target database paths must differ")
	}
	if err := requireStandaloneSQLiteSnapshot(sourcePath); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if _, err := os.Stat(targetPath); err == nil {
		return LegacyWorkspaceMigrationReport{}, fmt.Errorf("target database already exists: %s", targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), privateDirMode); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}

	sourceDB, err := openReadOnlySQLite(sourcePath)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	defer sourceDB.Close()

	ctx := context.Background()
	sourceTx, err := sourceDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	defer func() { _ = sourceTx.Rollback() }()

	plan, err := buildLegacyWorkspaceMigrationPlan(sourceTx, options.GlobalWorkspace, sourcePath)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}

	tempDir, err := os.MkdirTemp(filepath.Dir(targetPath), ".chatdock-workspace-migration-")
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	defer os.RemoveAll(tempDir)

	bootstrapStore, err := storepkg.NewStore(tempDir)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, fmt.Errorf("create current schema: %w", err)
	}
	if err := bootstrapStore.Close(); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	tempPath := filepath.Join(tempDir, "chatdock.sqlite")

	targetDB, err := sql.Open("sqlite3", tempPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	targetDB.SetMaxOpenConns(1)
	defer targetDB.Close()

	targetTx, err := targetDB.BeginTx(ctx, nil)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	defer func() { _ = targetTx.Rollback() }()

	report, err := executeLegacyWorkspaceMigration(sourceTx, targetTx, plan)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := targetTx.Commit(); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := sourceTx.Commit(); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}

	if err := validateMigratedWorkspaceDatabase(targetDB, plan, report); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := targetDB.Close(); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}

	sourceHashAfter, err := sha256File(sourcePath)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if sourceHashAfter != plan.SourceSHA256 {
		return LegacyWorkspaceMigrationReport{}, fmt.Errorf("source database changed during migration")
	}

	if err := os.Chmod(tempPath, privateFileMode); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := syncFile(tempPath); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	targetHash, err := sha256File(tempPath)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	// 临时文件位于目标目录下，硬链接发布既保证同一文件系统，也会在目标已存在时
	// 原子返回 EEXIST；不能使用会覆盖现有文件的 os.Rename。
	if err := os.Link(tempPath, targetPath); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := syncDirectory(filepath.Dir(targetPath)); err != nil {
		_ = os.Remove(targetPath)
		return LegacyWorkspaceMigrationReport{}, err
	}
	report.SourceSHA256 = plan.SourceSHA256
	report.TargetSHA256 = targetHash
	return report, nil
}

func buildLegacyWorkspaceMigrationPlan(reader sqlQueryer, globalWorkspace string, sourcePath string) (legacyWorkspaceMigrationPlan, error) {
	if err := validateLegacyWorkspaceSchema(reader); err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}
	if err := requireSQLiteCheck(reader, "quick_check"); err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}
	if err := requireNoForeignKeyIssues(reader); err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}

	sourceHash, err := sha256File(sourcePath)
	if err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}
	workspaces, err := loadLegacyWorkspaceRecords(reader)
	if err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}
	globalIndex := -1
	for i := range workspaces {
		if workspaces[i].Name == globalWorkspace {
			globalIndex = i
			break
		}
	}
	if globalIndex < 0 {
		return legacyWorkspaceMigrationPlan{}, fmt.Errorf("global workspace not found: %s", globalWorkspace)
	}

	globalWorkspaceRecord := workspaces[globalIndex]
	if strings.TrimSpace(globalWorkspaceRecord.ConfigRaw) == "" || strings.TrimSpace(globalWorkspaceRecord.Config.BaseURL) == "" || strings.TrimSpace(globalWorkspaceRecord.Config.Model) == "" {
		return legacyWorkspaceMigrationPlan{}, fmt.Errorf("global workspace model config is incomplete")
	}
	globalConfig := model.NormalizeModelConfig(globalWorkspaceRecord.Config)
	providers, err := modelprovider.LoadRecords(reader)
	if err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}
	counts, err := legacyMigrationTableCounts(reader)
	if err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}
	if err := validateLegacyGlobalKeys(reader); err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}

	mergedMCP, _, _, err := mergeLegacyWorkspaceMCPConfigs(workspaces, globalWorkspace)
	if err != nil {
		return legacyWorkspaceMigrationPlan{}, err
	}
	return legacyWorkspaceMigrationPlan{
		GlobalWorkspace: globalWorkspace,
		GlobalConfig:    globalConfig,
		GlobalMCP:       mergedMCP,
		Workspaces:      workspaces,
		Providers:       providers,
		TableCounts:     counts,
		SourceSHA256:    sourceHash,
	}, nil
}

func executeLegacyWorkspaceMigration(source sqlQueryer, target sqlQueryWriter, plan legacyWorkspaceMigrationPlan) (LegacyWorkspaceMigrationReport, error) {
	if _, err := target.Exec(`DELETE FROM global_settings`); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if _, err := target.Exec(`DELETE FROM meta`); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := copyNonLegacyMeta(source, target); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := modelprovider.SaveRecords(target, plan.Providers); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := upsertProviderFromConfigWith(target, plan.GlobalConfig); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	providers, err := modelprovider.LoadRecords(target)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}

	workspaceProviders, providers, providerReports, err := resolveLegacyWorkspaceProviders(source, plan, providers)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := modelprovider.SaveRecords(target, providers); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	globalConfig, err := applyProviderToConfigWith(target, plan.GlobalConfig)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := normalizeFallbackModelSelectionWith(target, &globalConfig); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	now := time.Now()
	if err := setGlobalJSONWith(target, "config", globalConfig, now); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	mcpContent, err := json.MarshalIndent(plan.GlobalMCP, "", "  ")
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := setGlobalRawWith(target, "mcp", string(mcpContent)+"\n", now); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}

	projects, err := insertLegacyProjects(target, plan)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	sessionReport, err := copyLegacySessions(source, target, plan, workspaceProviders, providers)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	counts, err := copyLegacyWorkspaceTables(source, target)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	counts["sessions"] = sessionReport.Count
	migrationMeta, err := json.Marshal(map[string]any{
		"global_workspace": plan.GlobalWorkspace,
		"source_sha256":    plan.SourceSHA256,
		"migrated_at":      now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	if err := setMetaValueWith(target, legacyWorkspaceMigrationMetaKey, string(migrationMeta)); err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}

	_, addedServers, deduplicatedServers, err := mergeLegacyWorkspaceMCPConfigs(plan.Workspaces, plan.GlobalWorkspace)
	if err != nil {
		return LegacyWorkspaceMigrationReport{}, err
	}
	return LegacyWorkspaceMigrationReport{
		GlobalWorkspace:         plan.GlobalWorkspace,
		Projects:                projects,
		LegacyProviders:         providerReports,
		AddedMCPServers:         addedServers,
		DeduplicatedMCPServers:  deduplicatedServers,
		RemappedProviderAliases: sessionReport.RemappedAliases,
		UnresolvedProviderIDs:   sessionReport.UnresolvedProviders,
		TableCounts:             counts,
		IgnoredLegacyTables:     existingIgnoredLegacyTables(source),
	}, nil
}
