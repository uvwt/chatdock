package chatdock

import (
	"chatdock/internal/chatdock/model"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Store) ListSkills() (model.SkillResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return model.SkillResponse{}, err
	}
	return model.SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) CreateSkill(input model.SaveSkillRequest) (model.SkillResponse, error) {
	next, err := normalizeSkillInput(input)
	if err != nil {
		return model.SkillResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return model.SkillResponse{}, err
	}
	for _, skill := range skills {
		if strings.EqualFold(skill.Name, next.Name) {
			return model.SkillResponse{}, fmt.Errorf("skill already exists: %s", next.Name)
		}
	}
	now := time.Now()
	next.ID = model.NewID()
	next.CreatedAt = now
	next.UpdatedAt = now
	skills = append(skills, next)
	if err := s.saveSkillsLocked(skills); err != nil {
		return model.SkillResponse{}, err
	}
	return model.SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) UpdateSkill(id string, input model.SaveSkillRequest) (model.SkillResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.SkillResponse{}, fmt.Errorf("skill id is empty")
	}
	next, err := normalizeSkillInput(input)
	if err != nil {
		return model.SkillResponse{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return model.SkillResponse{}, err
	}
	found := false
	for i := range skills {
		if skills[i].ID != id {
			if strings.EqualFold(skills[i].Name, next.Name) {
				return model.SkillResponse{}, fmt.Errorf("skill already exists: %s", next.Name)
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
		return model.SkillResponse{}, fmt.Errorf("skill not found: %s", id)
	}
	if err := s.saveSkillsLocked(skills); err != nil {
		return model.SkillResponse{}, err
	}
	return model.SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) DeleteSkill(id string) (model.SkillResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.SkillResponse{}, fmt.Errorf("skill id is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	skills, err := s.loadSkillsLocked()
	if err != nil {
		return model.SkillResponse{}, err
	}
	index := -1
	for i, skill := range skills {
		if skill.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return model.SkillResponse{}, fmt.Errorf("skill not found: %s", id)
	}
	skills = append(skills[:index], skills[index+1:]...)
	if err := s.saveSkillsLocked(skills); err != nil {
		return model.SkillResponse{}, err
	}
	return model.SkillResponse{Skills: cloneSkills(skills)}, nil
}

func (s *Store) enabledSkillsLocked() ([]model.Skill, error) {
	skills, err := s.loadSkillsLocked()
	if err != nil {
		return nil, err
	}
	out := make([]model.Skill, 0, len(skills))
	for _, skill := range skills {
		if skill.Enabled {
			out = append(out, skill)
		}
	}
	return cloneSkills(out), nil
}

func (s *Store) loadSkillsLocked() ([]model.Skill, error) {
	raw, ok, err := s.getPromptRawLocked(s.activePrompt, "skills")
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		skills := []model.Skill{}
		return skills, s.saveSkillsLocked(skills)
	}
	var skills []model.Skill
	if err := json.Unmarshal([]byte(raw), &skills); err != nil {
		return nil, fmt.Errorf("skills config must be valid json: %w", err)
	}
	sortSkills(skills)
	return skills, nil
}

func (s *Store) saveSkillsLocked(skills []model.Skill) error {
	sortSkills(skills)
	return s.setPromptJSONLocked(s.activePrompt, "skills", skills)
}

func sortSkills(skills []model.Skill) {
	sort.SliceStable(skills, func(i, j int) bool {
		if skills[i].Enabled != skills[j].Enabled {
			return skills[i].Enabled
		}
		return skills[i].UpdatedAt.After(skills[j].UpdatedAt)
	})
}

func normalizeSkillInput(input model.SaveSkillRequest) (model.Skill, error) {
	name, err := normalizeSkillName(input.Name)
	if err != nil {
		return model.Skill{}, err
	}
	desc := limitRunes(strings.TrimSpace(input.Description), 240)
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return model.Skill{}, fmt.Errorf("skill content is empty")
	}
	if len([]byte(content)) > 40000 {
		return model.Skill{}, fmt.Errorf("skill content is too large")
	}
	return model.Skill{Name: name, Description: desc, Content: content, Enabled: input.Enabled}, nil
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

func cloneSkills(skills []model.Skill) []model.Skill {
	out := make([]model.Skill, len(skills))
	copy(out, skills)
	return out
}
