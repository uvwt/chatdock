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

const maxProjectIDRunes = 128

func getGlobalRawWith(reader sqlQueryer, key string) (string, bool, error) {
	var value string
	err := reader.QueryRow(`SELECT value FROM global_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) getGlobalRawLocked(key string) (string, bool, error) {
	return getGlobalRawWith(s.db, key)
}

func (s *Store) setGlobalRawLocked(key string, value string) error {
	return setGlobalRawWith(s.db, key, value, time.Now())
}

func setGlobalRawWith(writer sqlWriter, key string, value string, now time.Time) error {
	_, err := writer.Exec(`INSERT INTO global_settings(key, value, updated_at) VALUES(?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, formatDBTime(now))
	return err
}

func setGlobalJSONWith(writer sqlWriter, key string, value any, now time.Time) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return setGlobalRawWith(writer, key, string(raw)+"\n", now)
}

func normalizeProjectID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("project id is empty")
	}
	if !utf8.ValidString(id) {
		return "", fmt.Errorf("project id is invalid")
	}
	if utf8.RuneCountInString(id) > maxProjectIDRunes {
		return "", fmt.Errorf("project id exceeds %d characters", maxProjectIDRunes)
	}
	if id == "." || id == ".." || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return "", fmt.Errorf("project id cannot contain path separators")
	}
	for _, r := range id {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("project id contains control characters")
		}
	}
	return id, nil
}
