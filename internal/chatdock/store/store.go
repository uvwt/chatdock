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

const (
	privateDirMode  = 0o700
	privateFileMode = 0o600
)

type Store struct {
	mu      sync.RWMutex
	dataDir string
	dbPath  string
	db      *sql.DB
}

func NewStore(dataDir string) (*Store, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		dataDir = "data"
	}
	if err := os.MkdirAll(dataDir, privateDirMode); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, "chatdock.sqlite")
	// 先以私有权限创建数据库文件；对已存在的数据库不做隐式 chmod 迁移。
	dbFile, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, privateFileMode)
	if err != nil {
		return nil, err
	}
	if err := dbFile.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{dataDir: dataDir, dbPath: dbPath, db: db}
	if err := store.initSQLite(); err != nil {
		return nil, closeStoreAfterInitError(db, "init sqlite", err)
	}
	if err := store.EnsureGlobalModelProviders(); err != nil {
		return nil, closeStoreAfterInitError(db, "ensure global model providers", err)
	}
	if err := store.recoverInterruptedWork(); err != nil {
		return nil, closeStoreAfterInitError(db, "recover interrupted work", err)
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
