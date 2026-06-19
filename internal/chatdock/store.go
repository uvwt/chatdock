package chatdock

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "github.com/mattn/go-sqlite3"
)

const defaultPromptName = "default"

type Store struct {
	mu           sync.RWMutex
	dataDir      string
	dbPath       string
	db           *sql.DB
	activePrompt string
	modelCfg     ModelConfig
	sessions     map[string]*Session
}

func NewStore(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, "chatdock.sqlite")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{
		dataDir:      dataDir,
		dbPath:       dbPath,
		db:           db,
		activePrompt: defaultPromptName,
		sessions:     make(map[string]*Session),
	}
	if err := store.initSQLite(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrateLegacyData(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.loadPromptLocked(defaultPromptName); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) initSQLite() error {
	stmts := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS prompts (name TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS prompt_kv (prompt TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(prompt, key), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS sessions (prompt TEXT NOT NULL, id TEXT NOT NULL, json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(prompt, id), FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_prompt_updated ON sessions(prompt, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_runs (prompt TEXT NOT NULL, id TEXT PRIMARY KEY, session_id TEXT NOT NULL, title TEXT NOT NULL, status TEXT NOT NULL, summary TEXT NOT NULL, error TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, event_count INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL, FOREIGN KEY(prompt) REFERENCES prompts(name) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_prompt_updated ON mcp_runs(prompt, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_runs_session_updated ON mcp_runs(session_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS mcp_run_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, server TEXT NOT NULL, tool TEXT NOT NULL, action TEXT NOT NULL, summary TEXT NOT NULL, arguments_json TEXT NOT NULL, result_json TEXT NOT NULL, error TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, started_at TEXT NOT NULL, finished_at TEXT NOT NULL, created_at TEXT NOT NULL, FOREIGN KEY(run_id) REFERENCES mcp_runs(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_run_seq ON mcp_run_events(run_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_run_events_tool_created ON mcp_run_events(tool, created_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ActivePrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activePrompt
}

func (s *Store) ListPrompts() (PromptResponse, error) {
	s.mu.RLock()
	active := s.activePrompt
	s.mu.RUnlock()

	prompts, err := s.listPrompts(active)
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: active, Prompts: prompts}, nil
}

func (s *Store) CreatePrompt(input CreatePromptRequest) (PromptResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return PromptResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return PromptResponse{}, err
	}
	if exists {
		return PromptResponse{}, fmt.Errorf("prompt already exists: %s", name)
	}

	now := time.Now()
	if err := s.insertPromptLocked(name, now); err != nil {
		return PromptResponse{}, err
	}
	cfg := s.modelCfg
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg = DefaultModelConfig()
	}
	cfg.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = DefaultModelConfig().SystemPrompt
	}
	cfg = NormalizeModelConfig(cfg)
	if err := s.setPromptJSONLocked(name, "config", cfg); err != nil {
		return PromptResponse{}, err
	}
	if err := s.setPromptRawLocked(name, "mcp", DefaultMCPConfig()); err != nil {
		return PromptResponse{}, err
	}

	if err := s.loadPromptLocked(name); err != nil {
		return PromptResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) DeletePrompt(input SelectPromptRequest) (PromptResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return PromptResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if name == defaultPromptName {
		return PromptResponse{}, fmt.Errorf("default workspace cannot be deleted")
	}
	if name == s.activePrompt {
		return PromptResponse{}, fmt.Errorf("active workspace cannot be deleted")
	}
	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return PromptResponse{}, err
	}
	if !exists {
		return PromptResponse{}, fmt.Errorf("prompt not found: %s", name)
	}
	names, err := s.listPromptNamesLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	if len(names) <= 1 {
		return PromptResponse{}, fmt.Errorf("last workspace cannot be deleted")
	}
	// prompt_kv 和 sessions 都声明了 ON DELETE CASCADE；删除 workspace 时必须只删 prompts 主表，
	// 让 SQLite 在同一个连接内级联清理关联数据，避免前后端各自补删造成状态不一致。
	if _, err := s.db.Exec(`DELETE FROM prompts WHERE name = ?`, name); err != nil {
		return PromptResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) SelectPrompt(input SelectPromptRequest) (PromptResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return PromptResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return PromptResponse{}, err
	}
	if !exists {
		return PromptResponse{}, fmt.Errorf("prompt not found: %s", name)
	}
	if err := s.loadPromptLocked(name); err != nil {
		return PromptResponse{}, err
	}
	prompts, err := s.listPromptsLocked()
	if err != nil {
		return PromptResponse{}, err
	}
	return PromptResponse{Active: s.activePrompt, Prompts: prompts}, nil
}

func (s *Store) GetMCPConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, ok, err := s.getPromptRawLocked(s.activePrompt, "mcp")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(content) == "" {
		content = DefaultMCPConfig()
		return content, s.setPromptRawLocked(s.activePrompt, "mcp", content)
	}
	return content, nil
}

func (s *Store) SaveMCPConfig(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		content = DefaultMCPConfig()
	}
	var probe any
	if err := json.Unmarshal([]byte(content), &probe); err != nil {
		return "", fmt.Errorf("mcp config must be valid json: %w", err)
	}
	pretty, err := prettyJSON(content)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return pretty, s.setPromptRawLocked(s.activePrompt, "mcp", pretty)
}

func DefaultMCPConfig() string {
	return `{
  "servers": {}
}
`
}

func (s *Store) GetModelConfig() ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelCfg
}

func (s *Store) SaveModelConfig(next ModelConfig) (ModelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
		next.APIKey = s.modelCfg.APIKey
	}

	s.modelCfg = NormalizeModelConfig(next)
	return s.modelCfg, s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
}

func (s *Store) ListSessions() []SessionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]SessionSummary, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, SessionSummary{
			ID:        session.ID,
			Title:     session.Title,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.UpdatedAt,
			Count:     len(session.Messages),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (s *Store) CreateSession() (*Session, error) {
	now := time.Now()
	session := &Session{
		ID:        NewID(),
		Title:     "新会话",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session
	return cloneSession(session), s.saveSessionLocked(session)
}

func (s *Store) GetSession(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	return cloneSession(session), true
}

func (s *Store) DeleteSession(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return false
	}
	delete(s.sessions, id)
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE prompt = ? AND id = ?`, s.activePrompt, id)
	_ = s.touchPromptLocked(s.activePrompt, time.Now())
	return true
}

func (s *Store) RenameSession(id string, title string) (*Session, error) {
	title = strings.TrimSpace(strings.ReplaceAll(title, "\n", " "))
	if title == "" {
		return nil, fmt.Errorf("session title is empty")
	}
	runes := []rune(title)
	if len(runes) > 80 {
		title = string(runes[:80])
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	session.Title = title
	session.UpdatedAt = time.Now()
	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) AppendUserMessage(sessionID string, content string) (*Session, ModelConfig, []Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ModelConfig{}, nil, ErrSessionNotFound
	}

	if session.Title == "新会话" || strings.TrimSpace(session.Title) == "" {
		session.Title = makeTitle(content)
	}

	now := time.Now()
	session.Messages = append(session.Messages, Message{Role: "user", Content: content, CreatedAt: now})
	session.UpdatedAt = now

	if err := s.saveSessionLocked(session); err != nil {
		return nil, ModelConfig{}, nil, err
	}

	cfg := s.modelCfg
	skills, err := s.enabledSkillsLocked()
	if err != nil {
		return nil, ModelConfig{}, nil, err
	}
	cfg.Skills = skills
	return cloneSession(session), cfg, cloneMessages(session.Messages), nil
}

func (s *Store) AppendAssistantMessage(sessionID string, content string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	now := time.Now()
	session.Messages = append(session.Messages, Message{Role: "assistant", Content: content, CreatedAt: now})
	session.UpdatedAt = now

	if err := s.saveSessionLocked(session); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Store) migrateLegacyData() error {
	migrated, err := s.metaValue("json_migrated")
	if err != nil {
		return err
	}
	if migrated == "1" {
		return s.ensurePromptLocked(defaultPromptName)
	}

	if err := s.ensurePromptLocked(defaultPromptName); err != nil {
		return err
	}
	legacyRoot := filepath.Join(s.dataDir, "prompts")
	if entries, err := os.ReadDir(legacyRoot); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				if err := s.importPromptDir(entry.Name(), filepath.Join(legacyRoot, entry.Name())); err != nil {
					return err
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.importLegacyDefaultFiles(); err != nil {
		return err
	}
	return s.setMetaValue("json_migrated", "1")
}

func (s *Store) importLegacyDefaultFiles() error {
	if _, ok, err := s.getPromptRawLocked(defaultPromptName, "config"); err != nil {
		return err
	} else if !ok {
		legacyConfig := filepath.Join(s.dataDir, "config.json")
		if raw, err := os.ReadFile(legacyConfig); err == nil {
			if err := s.setPromptRawLocked(defaultPromptName, "config", string(raw)); err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := s.setPromptJSONLocked(defaultPromptName, "config", DefaultModelConfig()); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	legacySessions := filepath.Join(s.dataDir, "sessions")
	if entries, err := os.ReadDir(legacySessions); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if err := s.importSessionFile(defaultPromptName, filepath.Join(legacySessions, entry.Name())); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) importPromptDir(name string, dir string) error {
	name, err := normalizePromptName(name)
	if err != nil {
		return nil
	}
	if err := s.ensurePromptLocked(name); err != nil {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		if err := s.setPromptRawLocked(name, "config", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "mcp.json")); err == nil {
		if err := s.setPromptRawLocked(name, "mcp", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "skills.json")); err == nil {
		if err := s.setPromptRawLocked(name, "skills", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "scheduled_tasks.json")); err == nil {
		if err := s.setPromptRawLocked(name, "scheduled_tasks", string(raw)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sessionsDir := filepath.Join(dir, "sessions")
	if entries, err := os.ReadDir(sessionsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if err := s.importSessionFile(name, filepath.Join(sessionsDir, entry.Name())); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) importSessionFile(prompt string, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil || session.ID == "" {
		return nil
	}
	return s.saveSessionForPromptLocked(prompt, &session)
}

func (s *Store) loadPromptLocked(name string) error {
	name, err := normalizePromptName(name)
	if err != nil {
		return err
	}
	if err := s.ensurePromptLocked(name); err != nil {
		return err
	}
	s.activePrompt = name
	s.sessions = make(map[string]*Session)
	if err := s.loadModelConfigLocked(); err != nil {
		return err
	}
	return s.loadSessionsLocked()
}

func (s *Store) loadModelConfigLocked() error {
	raw, ok, err := s.getPromptRawLocked(s.activePrompt, "config")
	if err != nil {
		return err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		s.modelCfg = DefaultModelConfig()
		return s.setPromptJSONLocked(s.activePrompt, "config", s.modelCfg)
	}
	if err := json.Unmarshal([]byte(raw), &s.modelCfg); err != nil {
		return err
	}
	s.modelCfg = NormalizeModelConfig(s.modelCfg)
	return nil
}

func (s *Store) loadSessionsLocked() error {
	rows, err := s.db.Query(`SELECT json FROM sessions WHERE prompt = ? ORDER BY updated_at DESC`, s.activePrompt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var session Session
		if err := json.Unmarshal([]byte(raw), &session); err != nil {
			return err
		}
		if session.ID != "" {
			s.sessions[session.ID] = &session
		}
	}
	return rows.Err()
}

func (s *Store) listPrompts(active string) ([]PromptSpace, error) {
	type promptRow struct {
		name       string
		createdRaw string
		updatedRaw string
	}
	rows, err := s.db.Query(`SELECT name, created_at, updated_at FROM prompts`)
	if err != nil {
		return nil, err
	}
	promptRows := []promptRow{}
	for rows.Next() {
		var row promptRow
		if err := rows.Scan(&row.name, &row.createdRaw, &row.updatedRaw); err != nil {
			_ = rows.Close()
			return nil, err
		}
		promptRows = append(promptRows, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	items := []PromptSpace{}
	for _, row := range promptRows {
		item, err := s.promptSummaryFromDB(row.name, active, row.createdRaw, row.updatedRaw)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == defaultPromptName {
			return true
		}
		if items[j].Name == defaultPromptName {
			return false
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *Store) listPromptNamesLocked() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM prompts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) listPromptsLocked() ([]PromptSpace, error) {
	return s.listPrompts(s.activePrompt)
}

func (s *Store) promptSummaryFromDB(name string, active string, createdRaw string, updatedRaw string) (PromptSpace, error) {
	createdAt := parseDBTime(createdRaw)
	updatedAt := parseDBTime(updatedRaw)
	var count int
	var latest sql.NullString
	if err := s.db.QueryRow(`SELECT COUNT(*), MAX(updated_at) FROM sessions WHERE prompt = ?`, name).Scan(&count, &latest); err != nil {
		return PromptSpace{}, err
	}
	if latest.Valid {
		if t := parseDBTime(latest.String); t.After(updatedAt) {
			updatedAt = t
		}
	}
	return PromptSpace{Name: name, Active: name == active, CreatedAt: createdAt, UpdatedAt: updatedAt, Count: count}, nil
}

func (s *Store) saveSessionLocked(session *Session) error {
	return s.saveSessionForPromptLocked(s.activePrompt, session)
}

func (s *Store) saveSessionForPromptLocked(prompt string, session *Session) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	now := time.Now()
	if err := s.ensurePromptLocked(prompt); err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO sessions(prompt, id, json, created_at, updated_at) VALUES(?, ?, ?, ?, ?)
ON CONFLICT(prompt, id) DO UPDATE SET json = excluded.json, updated_at = excluded.updated_at`, prompt, session.ID, string(raw)+"\n", formatDBTime(session.CreatedAt), formatDBTime(session.UpdatedAt))
	if err != nil {
		return err
	}
	return s.touchPromptLocked(prompt, now)
}

func (s *Store) promptExistsLocked(name string) (bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT name FROM prompts WHERE name = ?`, name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ensurePromptLocked(name string) error {
	exists, err := s.promptExistsLocked(name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.insertPromptLocked(name, time.Now())
}

func (s *Store) insertPromptLocked(name string, now time.Time) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO prompts(name, created_at, updated_at) VALUES(?, ?, ?)`, name, formatDBTime(now), formatDBTime(now))
	return err
}

func (s *Store) touchPromptLocked(name string, now time.Time) error {
	_, err := s.db.Exec(`UPDATE prompts SET updated_at = ? WHERE name = ?`, formatDBTime(now), name)
	return err
}

func (s *Store) getPromptRawLocked(prompt string, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM prompt_kv WHERE prompt = ? AND key = ?`, prompt, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) setPromptRawLocked(prompt string, key string, value string) error {
	if err := s.ensurePromptLocked(prompt); err != nil {
		return err
	}
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO prompt_kv(prompt, key, value, updated_at) VALUES(?, ?, ?, ?)
ON CONFLICT(prompt, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, prompt, key, value, formatDBTime(now))
	if err != nil {
		return err
	}
	return s.touchPromptLocked(prompt, now)
}

func (s *Store) setPromptJSONLocked(prompt string, key string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return s.setPromptRawLocked(prompt, key, string(raw)+"\n")
}

func (s *Store) metaValue(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) setMetaValue(key string, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func normalizePromptName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("prompt name is empty")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("prompt name is invalid")
	}
	if name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("prompt name cannot contain path separators")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("prompt name contains control characters")
		}
	}
	return name, nil
}

func writeRawFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func prettyJSON(content string) (string, error) {
	var value any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}

func formatDBTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.Format(time.RFC3339Nano)
}

func parseDBTime(value string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.Local()
	}
	return time.Now()
}

func cloneSession(session *Session) *Session {
	if session == nil {
		return nil
	}
	copySession := *session
	copySession.Messages = cloneMessages(session.Messages)
	return &copySession
}

func cloneMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}

func makeTitle(content string) string {
	content = strings.ReplaceAll(strings.TrimSpace(content), "\n", " ")
	runes := []rune(content)
	if len(runes) > 24 {
		runes = runes[:24]
	}
	if len(runes) == 0 {
		return "新会话"
	}
	return string(runes)
}
