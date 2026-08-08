package store

import (
	"sort"

	"chatdock/internal/model"
	"chatdock/internal/schedule"
)

// ListPinnedFeed returns all pinned sessions, projects and scheduled tasks in one shot.
func (s *Store) ListPinnedFeed() (model.PinnedFeedResponse, error) {
	sessions, err := s.ListSessions(SessionProjectFilter{
		Mode:   SessionProjectFilterAll,
		Pinned: SessionPinnedOnly(),
	})
	if err != nil {
		return model.PinnedFeedResponse{}, err
	}
	if sessions == nil {
		sessions = []model.SessionSummary{}
	}

	s.mu.RLock()
	projects, err := listProjectsWith(s.db)
	s.mu.RUnlock()
	if err != nil {
		return model.PinnedFeedResponse{}, err
	}
	pinnedProjects := make([]model.ProjectSummary, 0)
	for _, project := range projects {
		if project.Pinned {
			pinnedProjects = append(pinnedProjects, project)
		}
	}

	s.mu.Lock()
	tasks, err := s.loadScheduledTasksLocked()
	s.mu.Unlock()
	if err != nil {
		return model.PinnedFeedResponse{}, err
	}
	pinnedTasks := make([]model.ScheduledTask, 0)
	for _, task := range tasks {
		if task.Pinned {
			pinnedTasks = append(pinnedTasks, task)
		}
	}
	sort.SliceStable(pinnedTasks, func(i, j int) bool {
		if !pinnedTasks[i].UpdatedAt.Equal(pinnedTasks[j].UpdatedAt) {
			return pinnedTasks[i].UpdatedAt.After(pinnedTasks[j].UpdatedAt)
		}
		return pinnedTasks[i].Title < pinnedTasks[j].Title
	})

	return model.PinnedFeedResponse{
		Sessions: sessions,
		Projects: pinnedProjects,
		Tasks:    schedule.CloneTasks(pinnedTasks),
	}, nil
}
