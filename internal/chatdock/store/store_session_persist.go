package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

func (s *Store) loadSessionsLocked() error {
	rows, err := s.db.Query(`SELECT json FROM sessions WHERE prompt = ? ORDER BY updated_at DESC`, s.activePrompt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var session model.Session
		if err := json.Unmarshal([]byte(raw), &session); err != nil {
			return err
		}
		if session.ID != "" {
			s.sessions[session.ID] = &session
		}
	}
	return rows.Err()
}

func (s *Store) saveSessionLocked(session *model.Session) error {
	return s.saveSessionForPromptLocked(s.activePrompt, session)
}

func (s *Store) saveSessionForPromptLocked(prompt string, session *model.Session) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	now := time.Now()
	if err := s.ensurePromptLocked(prompt); err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO sessions(prompt, id, json, created_at, updated_at) VALUES(?, ?, ?, ?, ?)
ON CONFLICT(prompt, id) DO UPDATE SET json = excluded.json, updated_at = excluded.updated_at`, prompt, session.ID, string(raw)+"\n", formatDBTime(session.CreatedAt), formatDBTime(session.UpdatedAt))
	if err != nil {
		return err
	}
	return s.touchPromptLocked(prompt, now)
}
