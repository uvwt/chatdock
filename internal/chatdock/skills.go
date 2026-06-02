package chatdock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Store) ListSkills() (SkillResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return SkillResponse{}, err
	}
	return SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) CreateSkill(input SaveSkillRequest) (SkillResponse, error) {
	next, err := normalizeSkillInput(input)
	if err != nil {
		return SkillResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return SkillResponse{}, err
	}
	for _, skill := range skills {
		if strings.EqualFold(skill.Name, next.Name) {
			return SkillResponse{}, fmt.Errorf("skill already exists: %s", next.Name)
		}
	}
	now := time.Now()
	next.ID = NewID()
	next.CreatedAt = now
	next.UpdatedAt = now
	skills = append(skills, next)
	if err := s.saveSkillsLocked(skills); err != nil {
		return SkillResponse{}, err
	}
	return SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) UpdateSkill(id string, input SaveSkillRequest) (SkillResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SkillResponse{}, fmt.Errorf("skill id is empty")
	}
	next, err := normalizeSkillInput(input)
	if err != nil {
		return SkillResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return SkillResponse{}, err
	}
	found := false
	for i := range skills {
		if skills[i].ID != id {
			if strings.EqualFold(skills[i].Name, next.Name) {
				return SkillResponse{}, fmt.Errorf("skill already exists: %s", next.Name)
			}
			continue
		}
		next.ID = skills[i].ID
		next.CreatedAt = skills[i].CreatedAt
		if next.CreatedAt.IsZero() {
			next.CreatedAt = time.Now()
		}
		next.UpdatedAt = time.Now()
		skills[i] = next
		found = true
	}
	if !found {
		return SkillResponse{}, fmt.Errorf("skill not found: %s", id)
	}
	if err := s.saveSkillsLocked(skills); err != nil {
		return SkillResponse{}, err
	}
	return SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) DeleteSkill(id string) (SkillResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SkillResponse{}, fmt.Errorf("skill id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return SkillResponse{}, err
	}
	index := -1
	for i, skill := range skills {
		if skill.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return SkillResponse{}, fmt.Errorf("skill not found: %s", id)
	}
	skills = append(skills[:index], skills[index+1:]...)
	if err := s.saveSkillsLocked(skills); err != nil {
		return SkillResponse{}, err
	}
	return SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) enabledSkillsLocked() ([]Skill, error) {
	skills, err := s.loadSkillsLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(skills))
	for _, skill := range skills {
		if skill.Enabled {
			out = append(out, skill)
		}
	}
	return cloneSkills(out), nil
}

func (s *Store) loadSkillsLocked() ([]Skill, error) {
	raw, err := os.ReadFile(s.skillsPath())
	if errors.Is(err, os.ErrNotExist) {
		skills := []Skill{}
		return skills, s.saveSkillsLocked(skills)
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []Skill{}, nil
	}
	var skills []Skill
	if err := json.Unmarshal(raw, &skills); err != nil {
		return nil, fmt.Errorf("skills config must be valid json: %w", err)
	}
	sortSkills(skills)
	return skills, nil
}

func (s *Store) saveSkillsLocked(skills []Skill) error {
	sortSkills(skills)
	return writeJSON(s.skillsPath(), skills)
}

func sortSkills(skills []Skill) {
	sort.SliceStable(skills, func(i, j int) bool {
		if skills[i].Enabled != skills[j].Enabled {
			return skills[i].Enabled
		}
		return skills[i].UpdatedAt.After(skills[j].UpdatedAt)
	})
}

func normalizeSkillInput(input SaveSkillRequest) (Skill, error) {
	name, err := normalizeSkillName(input.Name)
	if err != nil {
		return Skill{}, err
	}
	desc := limitRunes(strings.TrimSpace(input.Description), 240)
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return Skill{}, fmt.Errorf("skill content is empty")
	}
	if len([]byte(content)) > 40000 {
		return Skill{}, fmt.Errorf("skill content is too large")
	}
	return Skill{Name: name, Description: desc, Content: content, Enabled: input.Enabled}, nil
}

func normalizeSkillName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("skill name is empty")
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("skill name is invalid")
	}
	if name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("skill name cannot contain path separators")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("skill name contains control characters")
		}
	}
	return limitRunes(name, 80), nil
}

func limitRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func cloneSkills(skills []Skill) []Skill {
	out := make([]Skill, len(skills))
	copy(out, skills)
	return out
}
