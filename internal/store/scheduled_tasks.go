package store

import (
	"chatdock/internal/schedule"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/model"
)

const (
	scheduleTypeOnce     = schedule.TypeOnce
	scheduleTypeInterval = schedule.TypeInterval
	scheduleTypeCron     = schedule.TypeCron
)

func (s *Store) ListScheduledTasks() (model.ScheduledTaskResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	return model.ScheduledTaskResponse{Tasks: schedule.CloneTasks(tasks)}, nil
}

func (s *Store) CreateScheduledTask(input model.ScheduledTaskRequest) (model.ScheduledTaskResponse, error) {
	now := time.Now()
	next, err := schedule.NormalizeInput(input, nil, now)
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	for _, task := range tasks {
		if strings.EqualFold(task.Title, next.Title) {
			return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task already exists: %s", next.Title)
		}
	}
	next.ID = model.NewID()
	next.CreatedAt = now
	next.UpdatedAt = now
	tasks = append(tasks, next)
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	return model.ScheduledTaskResponse{Tasks: schedule.CloneTasks(tasks)}, nil
}

func (s *Store) UpdateScheduledTask(id string, input model.ScheduledTaskRequest) (model.ScheduledTaskResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task id is empty")
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	for i, task := range tasks {
		if i != index && strings.EqualFold(task.Title, input.Title) {
			return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task already exists: %s", input.Title)
		}
	}
	next, err := schedule.NormalizeInput(input, &tasks[index], now)
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	next.ID = tasks[index].ID
	next.Pinned = tasks[index].Pinned
	next.SessionID = tasks[index].SessionID
	next.Running = tasks[index].Running
	next.LastRunAt = tasks[index].LastRunAt
	next.LastStatus = tasks[index].LastStatus
	next.LastError = tasks[index].LastError
	next.CreatedAt = tasks[index].CreatedAt
	if next.CreatedAt.IsZero() {
		next.CreatedAt = now
	}
	next.UpdatedAt = now
	tasks[index] = next
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	return model.ScheduledTaskResponse{Tasks: schedule.CloneTasks(tasks)}, nil
}

func (s *Store) PinScheduledTask(id string, pinned bool) (model.ScheduledTaskResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	found := false
	for i := range tasks {
		if tasks[i].ID != id {
			continue
		}
		tasks[i].Pinned = pinned
		found = true
		break
	}
	if !found {
		return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	return model.ScheduledTaskResponse{Tasks: schedule.CloneTasks(tasks)}, nil
}

func (s *Store) DeleteScheduledTask(id string) (model.ScheduledTaskResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.loadScheduledTasksLocked()
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	index := -1
	for i, task := range tasks {
		if task.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return model.ScheduledTaskResponse{}, fmt.Errorf("scheduled task not found: %s", id)
	}
	tasks = append(tasks[:index], tasks[index+1:]...)
	if err := s.saveScheduledTasksLocked(tasks); err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	return model.ScheduledTaskResponse{Tasks: schedule.CloneTasks(tasks)}, nil
}
