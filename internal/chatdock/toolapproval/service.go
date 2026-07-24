package toolapproval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"chatdock/internal/chatdock/model"
	"chatdock/internal/chatdock/store"
)

var ErrNotActive = errors.New("tool approval is not active")

type Confirmation struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id,omitempty"`
	Tool        string         `json:"tool"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Status      string         `json:"status"`
	RequestedAt time.Time      `json:"requested_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
	Message     string         `json:"message,omitempty"`
	decision    chan bool
}

type Service struct {
	store  *store.Store
	mu     sync.Mutex
	active map[string]*Confirmation
}

func NewService(store *store.Store) *Service {
	return &Service{store: store, active: make(map[string]*Confirmation)}
}

func (s *Service) Request(ctx context.Context, sessionID, tool string, args map[string]any, emit func(string, any) error) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		_, ok, err := s.store.GetSession(sessionID)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrSessionNotFound
		}
	}
	confirmation := &Confirmation{
		ID:          model.NewID(),
		SessionID:   sessionID,
		Tool:        strings.TrimSpace(tool),
		Arguments:   args,
		Status:      "pending",
		RequestedAt: time.Now(),
		Message:     "工具需要人工确认后才能继续执行。",
		decision:    make(chan bool, 1),
	}
	if _, err := s.store.SaveMCPConfirmation(store.MCPConfirmationRecord{
		ID: confirmation.ID, SessionID: confirmation.SessionID, Tool: confirmation.Tool,
		Arguments: confirmation.Arguments, Status: confirmation.Status,
		RequestedAt: confirmation.RequestedAt, Message: confirmation.Message,
	}); err != nil {
		return err
	}
	s.mu.Lock()
	s.active[confirmation.ID] = confirmation
	s.mu.Unlock()

	if emit != nil {
		if err := emit("tool_confirmation_required", confirmation); err != nil {
			_, finishErr := s.Finish(confirmation.ID, "cancelled", false)
			return errors.Join(err, finishErr)
		}
	}

	expiryTimer := time.NewTimer(10 * time.Minute)
	defer expiryTimer.Stop()
	var approved bool
	select {
	case approved = <-confirmation.decision:
	case <-ctx.Done():
		_, finishErr := s.Finish(confirmation.ID, "cancelled", false)
		return errors.Join(ctx.Err(), finishErr)
	case <-expiryTimer.C:
		expiredErr := fmt.Errorf("mcp tool confirmation expired: %s", tool)
		_, finishErr := s.Finish(confirmation.ID, "expired", false)
		return errors.Join(expiredErr, finishErr)
	}
	if emit != nil {
		if err := emit("tool_confirmation_resolved", map[string]any{"id": confirmation.ID, "tool": tool, "approved": approved}); err != nil {
			return err
		}
	}
	if !approved {
		return fmt.Errorf("mcp tool denied by user: %s", tool)
	}
	return nil
}

func (s *Service) Finish(id, status string, approved bool) (Confirmation, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.active[id]
	if !ok {
		return Confirmation{}, fmt.Errorf("%w: %s", ErrNotActive, id)
	}
	if item.Status != "pending" {
		delete(s.active, id)
		return *item, nil
	}
	persisted, err := s.store.ResolveMCPConfirmation(item.ID, status, approved, time.Now())
	if err != nil {
		return Confirmation{}, err
	}
	item.Status = persisted.Status
	item.ResolvedAt = persisted.ResolvedAt
	select {
	case item.decision <- approved:
	default:
	}
	resolved := *item
	delete(s.active, id)
	return resolved, nil
}

func (s *Service) List() ([]store.MCPConfirmationRecord, error) {
	return s.store.ListMCPConfirmations(true, 100)
}
