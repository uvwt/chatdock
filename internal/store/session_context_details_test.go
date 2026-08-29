package store

import (
	"encoding/json"
	"testing"

	"chatdock/internal/model"
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

func TestReplacingSessionMessageEventsRemovesStaleDetails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	event := model.MessageEvent{
		ID:      "event-to-replace",
		Kind:    "tool",
		Phase:   "done",
		CallKey: `read_file::{"path":"/tmp/old"}`,
		Details: map[string]any{"tool": "read_file", "result": map[string]any{"content": "old"}},
	}
	_, messageID, err := store.UpsertAssistantMessageCheckpoint(
		session.ID,
		"assistant-message",
		"旧结果",
		"",
		[]model.MessagePart{{Kind: "tool", CallKey: event.CallKey, Event: &event}},
		[]model.MessageEvent{event},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var detailsBefore int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM session_message_event_details WHERE session_id = ?`, session.ID).Scan(&detailsBefore); err != nil {
		t.Fatal(err)
	}
	if detailsBefore != 1 {
		t.Fatalf("details before replacement = %d, want 1", detailsBefore)
	}

	if _, _, err := store.UpsertAssistantMessageCheckpoint(session.ID, messageID, "新结果", "", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	var detailsAfter int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM session_message_event_details WHERE session_id = ?`, session.ID).Scan(&detailsAfter); err != nil {
		t.Fatal(err)
	}
	if detailsAfter != 0 {
		t.Fatalf("stale event details after replacement = %d, want 0", detailsAfter)
	}
}

func TestAppendingMessagePreservesExistingLazyEventDetails(t *testing.T) {
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
		ID:      "event-to-preserve",
		Kind:    "tool",
		Phase:   "done",
		CallKey: `read_file::{"path":"/tmp/preserve"}`,
		Details: map[string]any{"tool": "read_file", "result": map[string]any{"content": "preserved"}},
	}
	if _, err := store.AppendAssistantMessageWithParts(
		session.ID,
		"第一条回复",
		"",
		[]model.MessagePart{{Kind: "tool", CallKey: event.CallKey, Event: &event}},
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
	if _, err := reopened.AppendAssistantMessage(session.ID, "第二条回复"); err != nil {
		t.Fatal(err)
	}
	var detailsCount int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM session_message_event_details WHERE session_id = ? AND event_id = ?`, session.ID, event.ID).Scan(&detailsCount); err != nil {
		t.Fatal(err)
	}
	if detailsCount != 1 {
		t.Fatalf("preserved event details count = %d, want 1", detailsCount)
	}
	loaded, ok, err := reopened.GetSession(session.ID)
	if err != nil || !ok {
		t.Fatalf("load session: ok=%v err=%v", ok, err)
	}
	messages := cloneMessages(loaded.Messages)
	if err := reopened.HydrateMessageEventDetails(session.ID, messages, []int{0}); err != nil {
		t.Fatal(err)
	}
	result, _ := messages[0].Events[0].Details["result"].(map[string]any)
	if result["content"] != "preserved" {
		t.Fatalf("hydrated preserved result = %#v", result)
	}
}

func TestNewStoreBackfillsLegacyMCPAppEventMeta(t *testing.T) {
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
		ID:    "legacy-app-event",
		Kind:  "tool",
		Phase: "done",
		Text:  "调用完成：NexusDock__agentdock_context",
		Meta:  `{"tool":"NexusDock__agentdock_context"}`,
		Details: map[string]any{
			"tool": "NexusDock__agentdock_context",
			"data": map[string]any{
				"mcp_app": map[string]any{
					"server":       "NexusDock",
					"resource_uri": "ui://agentdock/context",
					"mime_type":    "text/html;profile=mcp-app",
					"html":         "<html>large app</html>",
				},
			},
		},
	}
	if _, err := store.AppendAssistantMessageWithParts(session.ID, "done", "", nil, []model.MessageEvent{event}); err != nil {
		t.Fatal(err)
	}
	// 模拟修复前已经落盘的事件：完整 detail 存在，但 meta 只有 tool 名。
	if _, err := store.db.Exec(`UPDATE session_message_events SET meta = ? WHERE session_id = ? AND id = ?`, `{"tool":"NexusDock__agentdock_context"}`, session.ID, event.ID); err != nil {
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
	var metaRaw string
	if err := reopened.db.QueryRow(`SELECT meta FROM session_message_events WHERE session_id = ? AND id = ?`, session.ID, event.ID).Scan(&metaRaw); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
		t.Fatal(err)
	}
	data, _ := meta["data"].(map[string]any)
	app, _ := data["mcp_app"].(map[string]any)
	if app["server"] != "NexusDock" || app["resource_uri"] != "ui://agentdock/context" || app["mime_type"] != "text/html;profile=mcp-app" {
		t.Fatalf("backfilled app descriptor = %#v", app)
	}
	if app["html"] != nil {
		t.Fatalf("backfill must keep app HTML lazy, got %#v", app)
	}
	loaded, ok, err := reopened.GetSession(session.ID)
	if err != nil || !ok {
		t.Fatalf("get session after backfill: ok=%v err=%v", ok, err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Events) != 1 {
		t.Fatalf("unexpected loaded session: %#v", loaded.Messages)
	}
	if len(loaded.Messages[0].Events[0].Details) != 0 {
		t.Fatalf("full event details must remain lazy after backfill: %#v", loaded.Messages[0].Events[0].Details)
	}
}
