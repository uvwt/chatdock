package store

import (
	"database/sql"
	"strings"
)

func (s *Store) upgradeSessionPromptSnapshots() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.modelConfigLocked()
	if err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT s.id, s.project_id, s.system_prompt_snapshot, s.project_prompt_snapshot, s.system_prompt_frozen, s.project_prompt_frozen, COALESCE(p.prompt, '') FROM sessions AS s LEFT JOIN projects AS p ON p.id = s.project_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type snapshotUpdate struct {
		id, system, project         string
		systemFrozen, projectFrozen bool
	}
	updates := make([]snapshotUpdate, 0)
	for rows.Next() {
		var id, system, project string
		var projectID sql.NullString
		var systemFrozen, projectFrozen int
		var projectPrompt string
		if err := rows.Scan(&id, &projectID, &system, &project, &systemFrozen, &projectFrozen, &projectPrompt); err != nil {
			return err
		}
		projectIDValue := ""
		if projectID.Valid {
			projectIDValue = strings.TrimSpace(projectID.String)
		}
		if systemFrozen != 0 && (projectIDValue == "" || projectFrozen != 0) {
			continue
		}
		if strings.TrimSpace(system) == "" {
			system = cfg.SystemPrompt
		}
		if projectIDValue != "" && strings.TrimSpace(project) == "" {
			project = projectPrompt
		}
		updates = append(updates, snapshotUpdate{id: id, system: system, project: project, systemFrozen: true, projectFrozen: projectIDValue != ""})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, update := range updates {
		if _, err := tx.Exec(`UPDATE sessions SET system_prompt_snapshot = ?, project_prompt_snapshot = ?, system_prompt_frozen = ?, project_prompt_frozen = ?, updated_at = updated_at WHERE id = ?`, update.system, update.project, boolInt(update.systemFrozen), boolInt(update.projectFrozen), update.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
