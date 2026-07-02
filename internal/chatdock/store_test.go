package chatdock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreSessionRenameAndExport(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(s.ID, "hello world"); err != nil {
		t.Fatal(err)
	}
	renamed, err := store.RenameSession(s.ID, "new title")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "new title" {
		t.Fatalf("unexpected title: %s", renamed.Title)
	}
	md := sessionToMarkdown(renamed)
	if !strings.Contains(md, "hello world") || !strings.Contains(md, "new title") {
		t.Fatalf("bad markdown export: %s", md)
	}
}

func TestStoreSessionSummaryPreviewAndClone(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(session.ID, "这是一条可以被会话搜索命中的用户消息"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAssistantMessage(session.ID, strings.Repeat("助手总结 ", 30)); err != nil {
		t.Fatal(err)
	}

	summaries := store.ListSessions()
	if len(summaries) != 1 {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
	if summaries[0].Preview == "" || summaries[0].LastRole != "assistant" || len([]rune(summaries[0].Preview)) > 121 {
		t.Fatalf("bad summary preview: %#v", summaries[0])
	}

	cloned, err := store.CloneSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.ID == session.ID || !strings.Contains(cloned.Title, "副本") || len(cloned.Messages) != 2 {
		t.Fatalf("bad cloned session: %#v", cloned)
	}
	summaries = store.ListSessions()
	if len(summaries) != 2 {
		t.Fatalf("clone should appear in session list: %#v", summaries)
	}
}

func TestStoreSkillsInjectedIntoChatConfig(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSkill(SaveSkillRequest{Name: "代码审查", Description: "审查 Go 代码", Content: "指出风险并给出修改建议。", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Skills) != 1 || !created.Skills[0].Enabled {
		t.Fatalf("unexpected skills: %#v", created.Skills)
	}
	s, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	_, cfg, _, err := store.AppendUserMessage(s.ID, "检查这段代码")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0].Name != "代码审查" {
		t.Fatalf("skill not injected into config: %#v", cfg.Skills)
	}
	prompt := buildSystemPrompt(cfg)
	if !strings.Contains(prompt, "# 已启用技能") || !strings.Contains(prompt, "指出风险并给出修改建议") {
		t.Fatalf("skill not injected into system prompt: %s", prompt)
	}
}

func TestStorePinSessionSortsBeforeRecentUnpinned(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	older, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	newer, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PinSession(older.ID, true); err != nil {
		t.Fatal(err)
	}
	summaries := store.ListSessions()
	if len(summaries) != 2 || summaries[0].ID != older.ID || !summaries[0].Pinned || summaries[1].ID != newer.ID {
		t.Fatalf("pinned session should sort first: %#v", summaries)
	}
	if _, err := store.PinSession(older.ID, false); err != nil {
		t.Fatal(err)
	}
	summaries = store.ListSessions()
	if summaries[0].ID != newer.ID || summaries[0].Pinned {
		t.Fatalf("unpinned sessions should sort by updated_at: %#v", summaries)
	}
}

func TestStoreScheduledTaskLifecycle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runAt := time.Now().Add(time.Hour).Format("2006-01-02T15:04")
	created, err := store.CreateScheduledTask(ScheduledTaskRequest{Title: "日报", Prompt: "总结今天", Enabled: true, ScheduleType: "once", RunAt: runAt})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 || created.Tasks[0].ID == "" || created.Tasks[0].NextRunAt.IsZero() {
		t.Fatalf("unexpected created tasks: %#v", created.Tasks)
	}

	updated, err := store.UpdateScheduledTask(created.Tasks[0].ID, ScheduledTaskRequest{Title: "日报", Prompt: "总结今天", Enabled: true, ScheduleType: "interval", IntervalMinutes: 5})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Tasks[0].ScheduleType != "interval" || updated.Tasks[0].IntervalMinutes != 5 {
		t.Fatalf("unexpected updated tasks: %#v", updated.Tasks)
	}

	deleted, err := store.DeleteScheduledTask(created.Tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Tasks) != 0 {
		t.Fatalf("unexpected deleted tasks: %#v", deleted.Tasks)
	}
}

func TestStoreDeleteWorkspaceCascadesPromptData(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreatePrompt(CreatePromptRequest{Name: "research", SystemPrompt: "研究空间"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveModelConfig(ModelConfig{BaseURL: "https://example.test/v1", Model: "demo", SystemPrompt: "研究助手"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMCPConfig(`{"servers":{}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSkill(SaveSkillRequest{Name: "研究技能", Content: "只做研究总结。", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateScheduledTask(ScheduledTaskRequest{Title: "研究任务", Prompt: "总结研究", Enabled: true, ScheduleType: "interval", IntervalMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AppendUserMessage(session.ID, "hello research"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SelectPrompt(SelectPromptRequest{Name: "default"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeletePrompt(SelectPromptRequest{Name: "research"}); err != nil {
		t.Fatal(err)
	}

	assertResearchPromptKVDeleted(t, store)
	assertResearchSessionsDeleted(t, store)
	if _, err := store.WorkspaceConfig("research"); err == nil {
		t.Fatal("deleted workspace config should fail")
	}
	if store.ActivePrompt() != "default" {
		t.Fatalf("active prompt changed after deleting inactive workspace: %s", store.ActivePrompt())
	}
}

func assertResearchPromptKVDeleted(t *testing.T, store *Store) {
	t.Helper()
	var got int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM prompt_kv WHERE prompt = ?", "research").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("prompt_kv rows for deleted workspace = %d, want 0", got)
	}
}

func assertResearchSessionsDeleted(t *testing.T, store *Store) {
	t.Helper()
	var got int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE prompt = ?", "research").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("session rows for deleted workspace = %d, want 0", got)
	}
}

func TestDataStatusReportsLatestBackup(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldBackup := filepath.Join(backupDir, "chatdock.sqlite.20260620-010000.bak")
	newBackup := filepath.Join(backupDir, "chatdock.sqlite.20260620-020000.bak")
	ignoredEnvBackup := filepath.Join(backupDir, ".env.20260620-030000.bak")
	ignoredComposeBackup := filepath.Join(backupDir, "compose.yaml.20260620-030000.bak")
	if err := os.WriteFile(oldBackup, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBackup, []byte("new-backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignoredEnvBackup, []byte("env"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignoredComposeBackup, []byte("compose"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-3 * time.Hour)
	newTime := time.Now().Add(-2 * time.Hour)
	ignoredTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldBackup, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newBackup, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(ignoredEnvBackup, ignoredTime, ignoredTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(ignoredComposeBackup, ignoredTime, ignoredTime); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.DataStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.BackupDir != backupDir || status.BackupCount != 2 || status.LatestBackupPath != newBackup || status.LatestBackupSizeBytes != int64(len("new-backup")) {
		t.Fatalf("unexpected backup status: %#v", status)
	}
	if len(status.BackupCheckedDirs) != 2 || status.BackupCheckedDirs[0] != backupDir || status.BackupCheckedDirs[1] != filepath.Join(dataDir, "backups") {
		t.Fatalf("unexpected backup checked dirs: %#v", status.BackupCheckedDirs)
	}
	if !status.BackupHealthy || status.BackupWarning != "" || status.LatestBackupAgeSeconds <= 0 {
		t.Fatalf("unexpected backup health status: %#v", status)
	}
	if len(status.Backups) != 2 || status.Backups[0].Path != newBackup || status.Backups[0].Name != "chatdock.sqlite.20260620-020000.bak" || status.Backups[1].Path != oldBackup {
		t.Fatalf("unexpected backup list: %#v", status.Backups)
	}
	if status.Backups[0].AgeSeconds <= 0 || status.Backups[1].AgeSeconds <= status.Backups[0].AgeSeconds {
		t.Fatalf("unexpected backup age list: %#v", status.Backups)
	}
	for _, backup := range status.Backups {
		if strings.HasPrefix(backup.Name, ".env") || strings.HasPrefix(backup.Name, "compose.yaml") {
			t.Fatalf("non-database backup should not be reported: %#v", status.Backups)
		}
	}
}

func TestDataStatusWarnsWhenBackupIsMissingOrStale(t *testing.T) {
	missingStore, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing, err := missingStore.DataStatus()
	if err != nil {
		t.Fatal(err)
	}
	if missing.BackupHealthy || missing.BackupWarning != "未检测到数据库备份" {
		t.Fatalf("missing backup should warn: %#v", missing)
	}

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleBackup := filepath.Join(backupDir, "chatdock.sqlite.20260617-010000.bak")
	if err := os.WriteFile(staleBackup, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(staleBackup, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}
	staleStore, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := staleStore.DataStatus()
	if err != nil {
		t.Fatal(err)
	}
	if stale.BackupHealthy || stale.BackupWarning != "最近数据库备份超过 48 小时" || stale.LatestBackupAgeSeconds < int64(70*time.Hour/time.Second) {
		t.Fatalf("stale backup should warn: %#v", stale)
	}
}

func TestStoreCloseClosesSQLiteConnection(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if store.db != nil {
		t.Fatal("closed store should clear sqlite connection")
	}
}

func TestStoreMCPRunsAndAgentTasks(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartMCPRun("session-1", "test run")
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"action": "create", "title": "ChatDock MCP"}
	result := map[string]any{"ok": true, "task_id": "task-1", "status": "active"}
	if _, err := store.AddMCPRunEvent(run.ID, runEventInput{Kind: "tool_call", Status: "success", Tool: "DockMini__task_manage", Arguments: args, Result: result}); err != nil {
		t.Fatal(err)
	}
	finished, err := store.FinishMCPRun(run.ID, "success", "done", nil)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "success" || finished.EventCount != 1 {
		t.Fatalf("unexpected finished run: %#v", finished)
	}
	runs, err := store.ListMCPRuns("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 || runs.Runs[0].ID != run.ID {
		t.Fatalf("unexpected runs: %#v", runs.Runs)
	}
	detail, err := store.MCPRunDetail(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != 1 || detail.Events[0].Tool != "task_manage" || detail.Events[0].Server != "DockMini" {
		t.Fatalf("unexpected run detail: %#v", detail)
	}
	tasks, err := store.ListAgentTasks(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].ID != "task-1" || tasks.Tasks[0].Status != "active" {
		t.Fatalf("unexpected agent tasks: %#v", tasks.Tasks)
	}
}

func TestStoreEffectiveMCPConfigFallsBackToDefaultWorkspace(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defaultConfig := `{"servers":{"agentdock":{"url":"http://host.docker.internal:18766/mcp"}}}`
	if _, err := store.SaveMCPConfig(defaultConfig); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePrompt(CreatePromptRequest{Name: "model", SystemPrompt: "model workspace"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMCPConfig(`{"servers":{}}`); err != nil {
		t.Fatal(err)
	}
	content, err := store.GetEffectiveMCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseMCPConfig(content)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Servers["agentdock"]; !ok {
		t.Fatalf("expected default workspace MCP server fallback, got %s", content)
	}
}

func TestStoreChatJobEventsPersistAndFinish(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	job, err := store.CreateChatJob(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddChatJobEvent(job.ID, "delta", StreamDelta{Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	loaded, events, err := store.ChatJobEventsAfter(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "running" || len(events) != 1 || events[0].Event != "delta" || events[0].Seq != 1 {
		t.Fatalf("unexpected loaded job/events: %#v %#v", loaded, events)
	}
	finished, err := store.FinishChatJob(job.ID, "success", "answer", "reason", nil)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "success" || finished.Answer != "answer" || finished.Reasoning != "reason" || finished.FinishedAt == nil {
		t.Fatalf("unexpected finished job: %#v", finished)
	}
}
