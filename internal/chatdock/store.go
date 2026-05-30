package chatdock

import (
	"bytes"
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
)

const defaultPromptName = "default"

type Store struct {
	mu           sync.RWMutex
	dataDir      string
	activePrompt string
	modelCfg     ModelConfig
	sessions     map[string]*Session
}

func NewStore(dataDir string) (*Store, error) {
	store := &Store{
		dataDir:      dataDir,
		activePrompt: defaultPromptName,
		sessions:     make(map[string]*Session),
	}

	if err := store.migrateLegacyData(); err != nil {
		return nil, err
	}
	if err := store.loadPromptLocked(defaultPromptName); err != nil {
		return nil, err
	}
	return store, nil
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

	if _, err := os.Stat(s.promptDir(name)); err == nil {
		return PromptResponse{}, fmt.Errorf("prompt already exists: %s", name)
	} else if !errors.Is(err, os.ErrNotExist) {
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

	if err := os.MkdirAll(s.promptSessionsDir(name), 0o755); err != nil {
		return PromptResponse{}, err
	}
	if err := writeJSON(s.promptConfigPath(name), cfg); err != nil {
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

func (s *Store) SelectPrompt(input SelectPromptRequest) (PromptResponse, error) {
	name, err := normalizePromptName(input.Name)
	if err != nil {
		return PromptResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.promptDir(name)); errors.Is(err, os.ErrNotExist) {
		return PromptResponse{}, fmt.Errorf("prompt not found: %s", name)
	} else if err != nil {
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

func (s *Store) GetMCPConfig() (string, error) {
	s.mu.RLock()
	path := s.mcpConfigPath()
	s.mu.RUnlock()

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		content := DefaultMCPConfig()
		return content, writeRawFile(path, content)
	}
	if err != nil {
		return "", err
	}
	return string(raw), nil
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
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, []byte(content), "", "  "); err != nil {
		return "", err
	}
	content = pretty.String() + "\n"

	s.mu.RLock()
	path := s.mcpConfigPath()
	s.mu.RUnlock()
	return content, writeRawFile(path, content)
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
	return s.modelCfg, writeJSON(s.configPath(), s.modelCfg)
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
	_ = os.Remove(s.sessionPath(id))
	return true
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

	return cloneSession(session), s.modelCfg, cloneMessages(session.Messages), nil
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
	if err := os.MkdirAll(s.promptsRoot(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.promptSessionsDir(defaultPromptName), 0o755); err != nil {
		return err
	}

	legacyConfig := filepath.Join(s.dataDir, "config.json")
	if _, err := os.Stat(s.promptConfigPath(defaultPromptName)); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(legacyConfig); err == nil {
			if err := copyFile(legacyConfig, s.promptConfigPath(defaultPromptName)); err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := writeJSON(s.promptConfigPath(defaultPromptName), DefaultModelConfig()); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	legacySessions := filepath.Join(s.dataDir, "sessions")
	entries, err := os.ReadDir(legacySessions)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		dst := filepath.Join(s.promptSessionsDir(defaultPromptName), entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := copyFile(filepath.Join(legacySessions, entry.Name()), dst); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadPromptLocked(name string) error {
	name, err := normalizePromptName(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.promptSessionsDir(name), 0o755); err != nil {
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
	path := s.configPath()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.modelCfg = DefaultModelConfig()
		return writeJSON(path, s.modelCfg)
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &s.modelCfg); err != nil {
		return err
	}
	s.modelCfg = NormalizeModelConfig(s.modelCfg)
	return nil
}

func (s *Store) loadSessionsLocked() error {
	entries, err := os.ReadDir(s.sessionsDir())
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.sessionsDir(), entry.Name()))
		if err != nil {
			return err
		}
		var session Session
		if err := json.Unmarshal(raw, &session); err != nil {
			return err
		}
		if session.ID != "" {
			s.sessions[session.ID] = &session
		}
	}
	return nil
}

func (s *Store) listPrompts(active string) ([]PromptSpace, error) {
	entries, err := os.ReadDir(s.promptsRoot())
	if err != nil {
		return nil, err
	}

	items := make([]PromptSpace, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		item, err := s.promptSummary(entry.Name(), active)
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

func (s *Store) listPromptsLocked() ([]PromptSpace, error) {
	return s.listPrompts(s.activePrompt)
}

func (s *Store) promptSummary(name string, active string) (PromptSpace, error) {
	info, err := os.Stat(s.promptDir(name))
	if err != nil {
		return PromptSpace{}, err
	}
	createdAt := info.ModTime()
	updatedAt := info.ModTime()
	if cfgInfo, err := os.Stat(s.promptConfigPath(name)); err == nil {
		createdAt = cfgInfo.ModTime()
		updatedAt = cfgInfo.ModTime()
	}
	count := 0
	entries, err := os.ReadDir(s.promptSessionsDir(name))
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				count++
				if sessionInfo, err := entry.Info(); err == nil && sessionInfo.ModTime().After(updatedAt) {
					updatedAt = sessionInfo.ModTime()
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return PromptSpace{}, err
	}
	return PromptSpace{Name: name, Active: name == active, CreatedAt: createdAt, UpdatedAt: updatedAt, Count: count}, nil
}

func (s *Store) saveSessionLocked(session *Session) error {
	return writeJSON(s.sessionPath(session.ID), session)
}

func (s *Store) promptsRoot() string {
	return filepath.Join(s.dataDir, "prompts")
}

func (s *Store) promptDir(name string) string {
	return filepath.Join(s.promptsRoot(), name)
}

func (s *Store) promptConfigPath(name string) string {
	return filepath.Join(s.promptDir(name), "config.json")
}

func (s *Store) promptMCPConfigPath(name string) string {
	return filepath.Join(s.promptDir(name), "mcp.json")
}

func (s *Store) promptSessionsDir(name string) string {
	return filepath.Join(s.promptDir(name), "sessions")
}

func (s *Store) configPath() string {
	return s.promptConfigPath(s.activePrompt)
}

func (s *Store) mcpConfigPath() string {
	return s.promptMCPConfigPath(s.activePrompt)
}

func (s *Store) sessionsDir() string {
	return s.promptSessionsDir(s.activePrompt)
}

func (s *Store) sessionPath(id string) string {
	return filepath.Join(s.sessionsDir(), id+".json")
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
