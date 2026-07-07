package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Store) workspaceExistsLocked(name string) (bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT name FROM workspaces WHERE name = ?`, name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ensureWorkspaceLocked(name string) error {
	exists, err := s.workspaceExistsLocked(name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.insertWorkspaceLocked(name, time.Now())
}

func (s *Store) insertWorkspaceLocked(name string, now time.Time) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO workspaces(name, created_at, updated_at) VALUES(?, ?, ?)`, name, formatDBTime(now), formatDBTime(now))
	return err
}

func (s *Store) touchWorkspaceLocked(name string, now time.Time) error {
	_, err := s.db.Exec(`UPDATE workspaces SET updated_at = ? WHERE name = ?`, formatDBTime(now), name)
	return err
}

func (s *Store) getWorkspaceRawLocked(prompt string, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM workspace_kv WHERE workspace_id = ? AND key = ?`, prompt, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) setWorkspaceRawLocked(prompt string, key string, value string) error {
	if err := s.ensureWorkspaceLocked(prompt); err != nil {
		return err
	}
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO workspace_kv(workspace_id, key, value, updated_at) VALUES(?, ?, ?, ?)
ON CONFLICT(workspace_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, prompt, key, value, formatDBTime(now))
	if err != nil {
		return err
	}
	return s.touchWorkspaceLocked(prompt, now)
}

func (s *Store) setWorkspaceJSONLocked(prompt string, key string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return s.setWorkspaceRawLocked(prompt, key, string(raw)+"\n")
}

func normalizeWorkspaceID(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("workspace id is empty")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("workspace id is invalid")
	}
	if name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("workspace id cannot contain path separators")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("workspace id contains control characters")
		}
	}
	return name, nil
}
