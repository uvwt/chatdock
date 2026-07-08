package store

import (
	"chatdock/internal/chatdock/model"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

const defaultWorkspaceID = "default"

type Store struct {
	mu               sync.RWMutex
	dataDir          string
	dbPath           string
	db               *sql.DB
	workspaceCacheID string
	modelCfg         model.ModelConfig
	sessions         map[string]*model.Session
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
		dataDir:          dataDir,
		dbPath:           dbPath,
		db:               db,
		workspaceCacheID: defaultWorkspaceID,
		sessions:         make(map[string]*model.Session),
	}
	if err := store.initSQLite(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init sqlite: %w", err)
	}
	if err := store.migrateLegacyData(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate legacy data: %w", err)
	}
	if err := store.migrateScheduledJSONToTables(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate scheduled tasks: %w", err)
	}
	if err := store.migrateSessionJSONToTables(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sessions: %w", err)
	}
	if err := store.migrateAttachmentBlobs(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate attachment blobs: %w", err)
	}
	if err := store.migrateToolEmbeddingBlobs(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate tool embedding blobs: %w", err)
	}
	if err := store.EnsureGlobalModelProviders(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure global model providers: %w", err)
	}
	if err := store.MarkRunningChatJobsInterrupted(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mark running chat jobs interrupted: %w", err)
	}
	if err := store.loadWorkspaceLocked(defaultWorkspaceID); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load default workspace: %w", err)
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
