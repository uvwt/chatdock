package chatdock

import (
	"context"
	"errors"
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestRequestMCPConfirmationCancelsWhenRequiredEventFails(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	emitErr := errors.New("persist required event")
	err = app.requestMCPConfirmation(context.Background(), "", "demo_tool", map[string]any{"value": 1}, func(event string, value any) error {
		if event != "tool_confirmation_required" {
			t.Fatalf("unexpected event %q", event)
		}
		return emitErr
	})
	if !errors.Is(err, emitErr) {
		t.Fatalf("expected emit error, got %v", err)
	}
	items, err := app.store.ListMCPConfirmations(true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != "cancelled" || items[0].ResolvedAt == nil {
		t.Fatalf("required event failure should cancel persisted confirmation: %#v", items)
	}
	app.confirmMu.Lock()
	activeCount := len(app.confirmations)
	app.confirmMu.Unlock()
	if activeCount != 0 {
		t.Fatalf("cancelled confirmation should leave no active state: %d", activeCount)
	}
}

func TestRequestMCPConfirmationResolvesApprovedState(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	required := make(chan string, 1)
	result := make(chan error, 1)
	go func() {
		result <- app.requestMCPConfirmation(context.Background(), "", "demo_tool", nil, func(event string, value any) error {
			if event == "tool_confirmation_required" {
				confirmation := value.(*MCPConfirmation)
				required <- confirmation.ID
			}
			return nil
		})
	}()
	id := <-required
	resolved, err := app.finishMCPConfirmation(id, "approved", true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "approved" || resolved.ResolvedAt == nil {
		t.Fatalf("unexpected resolved confirmation: %#v", resolved)
	}
	if err := <-result; err != nil {
		t.Fatalf("approved confirmation should allow tool execution: %v", err)
	}
	app.confirmMu.Lock()
	activeCount := len(app.confirmations)
	app.confirmMu.Unlock()
	if activeCount != 0 {
		t.Fatalf("resolved confirmation should leave no active state: %d", activeCount)
	}
}

func TestRequestMCPConfirmationRejectsUnknownSession(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	err = app.requestMCPConfirmation(context.Background(), "missing-session", "demo_tool", nil, nil)
	if err == nil {
		t.Fatal("unknown session must not create a confirmation")
	}
	items, listErr := app.store.ListMCPConfirmations(true, 10)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(items) != 0 {
		t.Fatalf("unknown session created confirmation: %#v", items)
	}
}
