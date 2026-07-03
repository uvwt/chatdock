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
