package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

const defaultWorkspaceID = "default"

type Store struct {
	mu      sync.RWMutex
	dataDir string
	dbPath  string
	db      *sql.DB
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
	store := &Store{dataDir: dataDir, dbPath: dbPath, db: db}
	if err := store.initSQLite(); err != nil {
		return nil, closeStoreAfterInitError(db, "init sqlite", err)
	}
	if err := store.migrateLegacyData(); err != nil {
		return nil, closeStoreAfterInitError(db, "migrate legacy data", err)
	}
	if err := store.migrateScheduledJSONToTables(); err != nil {
		return nil, closeStoreAfterInitError(db, "migrate scheduled tasks", err)
	}
	if err := store.migrateSessionJSONToTables(); err != nil {
		return nil, closeStoreAfterInitError(db, "migrate sessions", err)
	}
	if err := store.migrateAttachmentBlobs(); err != nil {
		return nil, closeStoreAfterInitError(db, "migrate attachment blobs", err)
	}
	if err := store.migrateToolEmbeddingBlobs(); err != nil {
		return nil, closeStoreAfterInitError(db, "migrate tool embedding blobs", err)
	}
	if err := store.migrateModelProviderAPIKeys(); err != nil {
		return nil, closeStoreAfterInitError(db, "migrate model provider api keys", err)
	}
	if err := store.EnsureGlobalModelProviders(); err != nil {
		return nil, closeStoreAfterInitError(db, "ensure global model providers", err)
	}
	if err := store.MarkRunningChatJobsInterrupted(); err != nil {
		return nil, closeStoreAfterInitError(db, "mark running chat jobs interrupted", err)
	}
	store.mu.Lock()
	err = store.ensureWorkspaceDefaultsLocked(defaultWorkspaceID)
	store.mu.Unlock()
	if err != nil {
		return nil, closeStoreAfterInitError(db, "ensure default workspace", err)
	}
	return store, nil
}

func closeStoreAfterInitError(db *sql.DB, stage string, initErr error) error {
	return errors.Join(fmt.Errorf("%s: %w", stage, initErr), db.Close())
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
