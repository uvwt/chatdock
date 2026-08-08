package toolapproval

import (
	"context"
	"errors"
	"testing"

	"chatdock/internal/store"
)

func newTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewService(st), st
}

func TestRequestCancelsWhenRequiredEventFails(t *testing.T) {
	service, st := newTestService(t)
	emitErr := errors.New("persist required event")
	err := service.Request(context.Background(), "", "demo_tool", map[string]any{"value": 1}, func(event string, value any) error {
		if event != "tool_confirmation_required" {
			t.Fatalf("unexpected event %q", event)
		}
		return emitErr
	})
	if !errors.Is(err, emitErr) {
		t.Fatalf("expected emit error, got %v", err)
	}
	items, err := st.ListMCPConfirmations(true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != "cancelled" || items[0].ResolvedAt == nil {
		t.Fatalf("required event failure should cancel persisted confirmation: %#v", items)
	}
	service.mu.Lock()
	activeCount := len(service.active)
	service.mu.Unlock()
	if activeCount != 0 {
		t.Fatalf("cancelled confirmation should leave no active state: %d", activeCount)
	}
}

func TestRequestResolvesApprovedState(t *testing.T) {
	service, _ := newTestService(t)
	required := make(chan string, 1)
	result := make(chan error, 1)
	go func() {
		result <- service.Request(context.Background(), "", "demo_tool", nil, func(event string, value any) error {
			if event == "tool_confirmation_required" {
				required <- value.(*Confirmation).ID
			}
			return nil
		})
	}()
	id := <-required
	resolved, err := service.Finish(id, "approved", true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "approved" || resolved.ResolvedAt == nil {
		t.Fatalf("unexpected resolved confirmation: %#v", resolved)
	}
	if err := <-result; err != nil {
		t.Fatalf("approved confirmation should allow tool execution: %v", err)
	}
	service.mu.Lock()
	activeCount := len(service.active)
	service.mu.Unlock()
	if activeCount != 0 {
		t.Fatalf("resolved confirmation should leave no active state: %d", activeCount)
	}
}

func TestRequestRejectsUnknownSession(t *testing.T) {
	service, st := newTestService(t)
	if err := service.Request(context.Background(), "missing-session", "demo_tool", nil, nil); err == nil {
		t.Fatal("unknown session must not create a confirmation")
	}
	items, err := st.ListMCPConfirmations(true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("unknown session created confirmation: %#v", items)
	}
}
