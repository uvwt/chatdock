package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/llm"
)

type ContextCheckpoint struct {
	SessionID          string    `json:"session_id"`
	ProviderID         string    `json:"provider_id"`
	Model              string    `json:"model"`
	Summary            string    `json:"summary"`
	CutoffMessageID    string    `json:"cutoff_message_id,omitempty"`
	CutoffMessageIndex int       `json:"cutoff_message_index"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ContextCheckpointStatus struct {
	Exists             bool      `json:"exists"`
	SummaryTokens      int       `json:"summary_tokens,omitempty"`
	CutoffMessageID    string    `json:"cutoff_message_id,omitempty"`
	CutoffMessageIndex int       `json:"cutoff_message_index,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

func saveContextCheckpointWith(tx sqlWriter, checkpoint ContextCheckpoint) error {
	checkpoint.SessionID = strings.TrimSpace(checkpoint.SessionID)
	checkpoint.ProviderID = strings.TrimSpace(checkpoint.ProviderID)
	checkpoint.Model = strings.TrimSpace(checkpoint.Model)
	if checkpoint.SessionID == "" || checkpoint.ProviderID == "" || checkpoint.Model == "" {
		return fmt.Errorf("context checkpoint identity is incomplete")
	}
	if checkpoint.CutoffMessageIndex < 0 {
		checkpoint.CutoffMessageIndex = -1
	}
	if checkpoint.UpdatedAt.IsZero() {
		checkpoint.UpdatedAt = time.Now()
	}
	_, err := tx.Exec(`INSERT INTO session_context_checkpoints(session_id, provider_id, model, summary, cutoff_message_id, cutoff_message_index, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id, provider_id, model) DO UPDATE SET summary = excluded.summary, cutoff_message_id = excluded.cutoff_message_id, cutoff_message_index = excluded.cutoff_message_index, updated_at = excluded.updated_at`, checkpoint.SessionID, checkpoint.ProviderID, checkpoint.Model, checkpoint.Summary, checkpoint.CutoffMessageID, checkpoint.CutoffMessageIndex, formatScheduleDBTime(checkpoint.UpdatedAt))
	return err
}

func deleteSessionContextCheckpointsWith(tx sqlWriter, sessionID string) error {
	_, err := tx.Exec(`DELETE FROM session_context_checkpoints WHERE session_id = ?`, strings.TrimSpace(sessionID))
	return err
}

func (s *Store) copySessionContextCheckpointsLocked(sourceID, targetID string) error {
	sourceID = strings.TrimSpace(sourceID)
	targetID = strings.TrimSpace(targetID)
	if sourceID == "" || targetID == "" {
		return fmt.Errorf("context checkpoint session id is empty")
	}
	_, err := s.db.Exec(`INSERT INTO session_context_checkpoints(session_id, provider_id, model, summary, cutoff_message_id, cutoff_message_index, updated_at)
SELECT ?, provider_id, model, summary, cutoff_message_id, cutoff_message_index, updated_at
FROM session_context_checkpoints WHERE session_id = ?`, targetID, sourceID)
	return err
}

func (s *Store) ContextCheckpointStatus(sessionID, providerID, modelName string) (ContextCheckpointStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.contextCheckpointStatusLocked(sessionID, providerID, modelName)
}

func (s *Store) GetContextCheckpoint(sessionID, providerID, modelName string) (ContextCheckpoint, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return getContextCheckpointLocked(s.db, sessionID, providerID, modelName)
}

func getContextCheckpointLocked(reader sqlQueryer, sessionID, providerID, modelName string) (ContextCheckpoint, bool, error) {
	var checkpoint ContextCheckpoint
	var updatedAt string
	err := reader.QueryRow(`SELECT session_id, provider_id, model, summary, cutoff_message_id, cutoff_message_index, updated_at FROM session_context_checkpoints WHERE session_id = ? AND provider_id = ? AND model = ?`, strings.TrimSpace(sessionID), strings.TrimSpace(providerID), strings.TrimSpace(modelName)).Scan(&checkpoint.SessionID, &checkpoint.ProviderID, &checkpoint.Model, &checkpoint.Summary, &checkpoint.CutoffMessageID, &checkpoint.CutoffMessageIndex, &updatedAt)
	if err == sql.ErrNoRows {
		return ContextCheckpoint{}, false, nil
	}
	if err != nil {
		return ContextCheckpoint{}, false, err
	}
	checkpoint.UpdatedAt = parseDBTimeZero(updatedAt)
	return checkpoint, true, nil
}

func (s *Store) contextCheckpointStatusLocked(sessionID, providerID, modelName string) (ContextCheckpointStatus, error) {
	var status ContextCheckpointStatus
	var summary, updatedAt string
	err := s.db.QueryRow(`SELECT summary, cutoff_message_id, cutoff_message_index, updated_at FROM session_context_checkpoints WHERE session_id = ? AND provider_id = ? AND model = ?`, strings.TrimSpace(sessionID), strings.TrimSpace(providerID), strings.TrimSpace(modelName)).Scan(&summary, &status.CutoffMessageID, &status.CutoffMessageIndex, &updatedAt)
	if err == sql.ErrNoRows {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Exists = true
	status.SummaryTokens = llm.EstimateTokens(summary)
	status.UpdatedAt = parseDBTimeZero(updatedAt)
	return status, nil
}

func (s *Store) SaveContextCheckpoint(checkpoint ContextCheckpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := saveContextCheckpointWith(tx, checkpoint); err != nil {
		return err
	}
	return tx.Commit()
}
