package chatdock

import (
	"chatdock/internal/chatdock/model"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

const defaultPromptName = "default"

type Store struct {
	mu           sync.RWMutex
	dataDir      string
	dbPath       string
	db           *sql.DB
	activePrompt string
	modelCfg     model.ModelConfig
	sessions     map[string]*model.Session
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
		sessions:     make(map[string]*model.Session),
	}
	if err := store.initSQLite(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrateLegacyData(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.MarkRunningChatJobsInterrupted(); err != nil {
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
