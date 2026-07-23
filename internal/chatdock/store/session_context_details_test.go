package store

import (
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestHydrateMessageEventDetailsReloadsLazyToolPayload(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	event := model.MessageEvent{
		ID:      "event-history-tool",
		Kind:    "tool",
		Phase:   "done",
		CallKey: `read_file::{"path":"/tmp/demo"}`,
		Text:    "调用完成：read_file",
		Details: map[string]any{
			"event":     "tool_call_result",
			"tool":      "read_file",
			"arguments": map[string]any{"path": "/tmp/demo"},
			"data": map[string]any{
				"ok":     true,
				"tool":   "read_file",
				"result": map[string]any{"content": "loaded detail"},
			},
		},
	}
	if _, err := store.AppendAssistantMessageWithParts(
		session.ID,
		"读取完成",
		"内部推理",
		[]model.MessagePart{{Kind: "tool", CallKey: event.CallKey, Event: &event}, {Kind: "text", Text: "读取完成"}},
		[]model.MessageEvent{event},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, ok, err := reopened.GetSession(session.ID)
	if err != nil || !ok {
		t.Fatalf("load session: ok=%v err=%v", ok, err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Events) != 1 {
		t.Fatalf("unexpected messages after reload: %#v", loaded.Messages)
	}
	if len(loaded.Messages[0].Events[0].Details) != 0 {
		t.Fatalf("event details should remain lazy after startup: %#v", loaded.Messages[0].Events[0].Details)
	}

	messages := cloneMessages(loaded.Messages)
	if err := reopened.HydrateMessageEventDetails(session.ID, messages, []int{0}); err != nil {
		t.Fatal(err)
	}
	data, _ := messages[0].Events[0].Details["data"].(map[string]any)
	result, _ := data["result"].(map[string]any)
	if result["content"] != "loaded detail" {
		t.Fatalf("hydrated result = %#v", result)
	}
}
