package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"chatdock/internal/model"
)

func (s *Store) ListProjects() (model.ProjectListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	projects, err := listProjectsWith(s.db)
	if err != nil {
		return model.ProjectListResponse{}, err
	}
	sessionCount, plainSessionCount, err := projectSessionCountsWith(s.db)
	if err != nil {
		return model.ProjectListResponse{}, err
	}
	return model.ProjectListResponse{
		Projects:          projects,
		SessionCount:      sessionCount,
		PlainSessionCount: plainSessionCount,
	}, nil
}

func (s *Store) CreateProject(input model.CreateProjectRequest) (model.Project, error) {
	name, err := normalizeProjectName(input.Name)
	if err != nil {
		return model.Project{}, err
	}
	now := time.Now()
	project := model.Project{ID: model.NewID(), Name: name, Prompt: strings.TrimSpace(input.Prompt), CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`INSERT INTO projects(id, name, prompt, pinned, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`, project.ID, project.Name, project.Prompt, boolInt(project.Pinned), formatDBTime(project.CreatedAt), formatDBTime(project.UpdatedAt)); err != nil {
		return model.Project{}, err
	}
	return project, nil
}

func (s *Store) UpdateProject(id string, input model.UpdateProjectRequest) (model.Project, error) {
	id, err := normalizeProjectID(id)
	if err != nil {
		return model.Project{}, err
	}
	name, err := normalizeProjectName(input.Name)
	if err != nil {
		return model.Project{}, err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`UPDATE projects SET name = ?, prompt = ?, updated_at = ? WHERE id = ?`, name, strings.TrimSpace(input.Prompt), formatDBTime(now), id)
	if err != nil {
		return model.Project{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return model.Project{}, err
	}
	if affected == 0 {
		return model.Project{}, fmt.Errorf("%w: %s", model.ErrProjectNotFound, id)
	}
	project, ok, err := projectByIDWith(s.db, id)
	if err != nil {
		return model.Project{}, err
	}
	if !ok {
		return model.Project{}, fmt.Errorf("%w: %s", model.ErrProjectNotFound, id)
	}
	return project, nil
}

func (s *Store) PinProject(id string, pinned bool) (model.Project, error) {
	id, err := normalizeProjectID(id)
	if err != nil {
		return model.Project{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`UPDATE projects SET pinned = ? WHERE id = ?`, boolInt(pinned), id)
	if err != nil {
		return model.Project{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return model.Project{}, err
	}
	if affected == 0 {
		return model.Project{}, fmt.Errorf("%w: %s", model.ErrProjectNotFound, id)
	}
	project, ok, err := projectByIDWith(s.db, id)
	if err != nil {
		return model.Project{}, err
	}
	if !ok {
		return model.Project{}, fmt.Errorf("%w: %s", model.ErrProjectNotFound, id)
	}
	return project, nil
}

func (s *Store) DeleteProject(id string) (bool, error) {
	id, err := normalizeProjectID(id)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) ProjectPrompt(id string) (string, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	project, ok, err := projectByIDWith(s.db, id)
	if err != nil || !ok {
		return "", ok, err
	}
	return project.Prompt, true, nil
}

func (s *Store) ProjectCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return projectCountWith(s.db)
}

func projectCountWith(reader sqlQueryer) (int, error) {
	var count int
	err := reader.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&count)
	return count, err
}

func listProjectsWith(reader sqlQueryer) ([]model.ProjectSummary, error) {
	rows, err := reader.Query(`
		SELECT p.id, p.name, p.prompt, p.pinned, p.created_at, p.updated_at, COUNT(s.id)
		FROM projects p
		LEFT JOIN sessions s ON s.project_id = p.id
		GROUP BY p.id, p.name, p.prompt, p.pinned, p.created_at, p.updated_at
		ORDER BY p.pinned DESC, p.updated_at DESC, p.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []model.ProjectSummary{}
	for rows.Next() {
		var summary model.ProjectSummary
		var pinned int
		var createdAt, updatedAt string
		if err := rows.Scan(&summary.ID, &summary.Name, &summary.Prompt, &pinned, &createdAt, &updatedAt, &summary.SessionCount); err != nil {
			return nil, err
		}
		summary.Pinned = pinned != 0
		summary.CreatedAt = parseDBTimeZero(createdAt)
		summary.UpdatedAt = parseDBTimeZero(updatedAt)
		projects = append(projects, summary)
	}
	return projects, rows.Err()
}

func projectSessionCountsWith(reader sqlQueryer) (int, int, error) {
	var total, plain int
	if err := reader.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN project_id IS NULL THEN 1 ELSE 0 END), 0) FROM sessions`).Scan(&total, &plain); err != nil {
		return 0, 0, err
	}
	return total, plain, nil
}

func projectByIDWith(reader sqlQueryer, id string) (model.Project, bool, error) {
	var project model.Project
	var pinned int
	var createdAt, updatedAt string
	err := reader.QueryRow(`SELECT id, name, prompt, pinned, created_at, updated_at FROM projects WHERE id = ?`, id).Scan(&project.ID, &project.Name, &project.Prompt, &pinned, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Project{}, false, nil
	}
	if err != nil {
		return model.Project{}, false, err
	}
	project.Pinned = pinned != 0
	project.CreatedAt = parseDBTimeZero(createdAt)
	project.UpdatedAt = parseDBTimeZero(updatedAt)
	return project, true, nil
}

func projectExistsWith(reader sqlQueryer, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return true, nil
	}
	var got string
	err := reader.QueryRow(`SELECT id FROM projects WHERE id = ?`, id).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func scanProject(scanner interface{ Scan(...any) error }) (model.Project, error) {
	var project model.Project
	var pinned int
	var createdAt, updatedAt string
	if err := scanner.Scan(&project.ID, &project.Name, &project.Prompt, &pinned, &createdAt, &updatedAt); err != nil {
		return model.Project{}, err
	}
	project.Pinned = pinned != 0
	project.CreatedAt = parseDBTimeZero(createdAt)
	project.UpdatedAt = parseDBTimeZero(updatedAt)
	return project, nil
}

func normalizeProjectName(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\n", " "))
	if name == "" {
		return "", fmt.Errorf("project name is empty")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("project name is invalid")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("project name contains control characters")
		}
	}
	return limitRunes(name, 80), nil
}
