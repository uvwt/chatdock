package httpapi

import (
	"testing"
	"time"

	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

func TestMCPConfirmationPersistsAndResolvesAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	app, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.store.SaveMCPConfirmation(storepkg.MCPConfirmationRecord{
		SessionID:   "s1",
		Tool:        "memory.write",
		Arguments:   map[string]any{"path": "note.md"},
		Status:      "pending",
		RequestedAt: time.Now(),
		Message:     "需要确认",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: dataDir, WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	items, err := restarted.store.ListMCPConfirmations(true, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID || items[0].Status != "pending" {
		t.Fatalf("confirmation was not persisted: %#v", items)
	}
	resolved, err := restarted.store.ResolveMCPConfirmation(created.ID, "approved", true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "approved" || resolved.ResolvedAt == nil {
		t.Fatalf("confirmation was not resolved: %#v", resolved)
	}
}
