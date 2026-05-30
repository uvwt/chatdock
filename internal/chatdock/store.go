package chatdock

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu       sync.RWMutex
	dataDir  string
	modelCfg ModelConfig
	sessions map[string]*Session
}

func NewStore(dataDir string) (*Store, error) {
	store := &Store{
		dataDir:  dataDir,
		sessions: make(map[string]*Session),
	}

	if err := os.MkdirAll(store.sessionsDir(), 0o755); err != nil {
		return nil, err
	}
	if err := store.loadModelConfig(); err != nil {
		return nil, err
	}
	if err := store.loadSessions(); err != nil {
		return nil, err
	}
	return store, nil
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

func (s *Store) loadModelConfig() error {
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

func (s *Store) loadSessions() error {
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

func (s *Store) saveSessionLocked(session *Session) error {
	return writeJSON(s.sessionPath(session.ID), session)
}

func (s *Store) configPath() string {
	return filepath.Join(s.dataDir, "config.json")
}

func (s *Store) sessionsDir() string {
	return filepath.Join(s.dataDir, "sessions")
}

func (s *Store) sessionPath(id string) string {
	return filepath.Join(s.sessionsDir(), id+".json")
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
