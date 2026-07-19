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

const maxWorkspaceIDRunes = 128

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

func touchWorkspace(writer sqlWriter, name string, now time.Time) error {
	result, err := writer.Exec(`UPDATE workspaces SET updated_at = ? WHERE name = ?`, formatDBTime(now), name)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) getWorkspaceRawLocked(prompt string, key string) (string, bool, error) {
	return getWorkspaceRawWith(s.db, prompt, key)
}

func getWorkspaceRawWith(reader sqlQueryer, prompt string, key string) (string, bool, error) {
	var value string
	err := reader.QueryRow(`SELECT value FROM workspace_kv WHERE workspace_id = ? AND key = ?`, prompt, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) setWorkspaceRawLocked(prompt string, key string, value string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO workspaces(name, created_at, updated_at) VALUES(?, ?, ?)`, prompt, formatDBTime(now), formatDBTime(now)); err != nil {
		return err
	}
	if err := setWorkspaceRawWith(tx, prompt, key, value, now); err != nil {
		return err
	}
	return tx.Commit()
}

func setWorkspaceRawWith(writer sqlWriter, prompt string, key string, value string, now time.Time) error {
	if _, err := writer.Exec(`INSERT INTO workspace_kv(workspace_id, key, value, updated_at) VALUES(?, ?, ?, ?)
ON CONFLICT(workspace_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, prompt, key, value, formatDBTime(now)); err != nil {
		return err
	}
	return touchWorkspace(writer, prompt, now)
}

func (s *Store) setWorkspaceJSONLocked(prompt string, key string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return s.setWorkspaceRawLocked(prompt, key, string(raw)+"\n")
}

func setWorkspaceJSONWith(writer sqlWriter, prompt string, key string, value any, now time.Time) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return setWorkspaceRawWith(writer, prompt, key, string(raw)+"\n", now)
}

func normalizeWorkspaceID(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("workspace id is empty")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("workspace id is invalid")
	}
	if utf8.RuneCountInString(name) > maxWorkspaceIDRunes {
		return "", fmt.Errorf("workspace id exceeds %d characters", maxWorkspaceIDRunes)
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
