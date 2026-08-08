package httpapi

import (
	"strings"
	"testing"

	"chatdock/internal/model"
)

func TestCompactSessionToolEventDetailsRemovesLegacyJSONMetaPayload(t *testing.T) {
	large := strings.Repeat("payload", 500)
	event := model.MessageEvent{
		Kind:    "tool",
		Phase:   "completed",
		CallKey: "call-1",
		Meta:    `{"tool":"demo","arguments":{"names":["one","two"]},"data":{"result":{"status":"ok","tools":[{"name":"alpha"}],"large":"` + large + `"}}}`,
	}
	session := &model.Session{ID: "session-1", Messages: []model.Message{{ID: "message-1", Events: []model.MessageEvent{event}}}}

	compacted := compactSessionToolEventDetails(session)
	got := compacted.Messages[0].Events[0]
	if got.Meta != "" {
		t.Fatalf("legacy JSON meta was retained: %d bytes", len(got.Meta))
	}
	if got.Details["lazy"] != true || got.Details["event_id"] == "" {
		t.Fatalf("lazy metadata missing: %#v", got.Details)
	}
	if strings.Contains(shortString(got.Details, 10_000), large) {
		t.Fatal("large legacy result leaked into compact details")
	}
	args := got.Details["arguments"].(map[string]any)
	names := args["names"].([]string)
	if len(names) != 2 || names[0] != "one" {
		t.Fatalf("[]string arguments were not preserved: %#v", names)
	}
	if session.Messages[0].Events[0].Meta == "" {
		t.Fatal("compaction mutated the original session")
	}
}

func TestCompactMessageEventBoundsPlainTextMeta(t *testing.T) {
	meta := strings.Repeat("说明", 400)
	got := compactMessageEvent("session", "message", 0, 0, -1, model.MessageEvent{Kind: "tool", Meta: meta})
	if len([]rune(got.Meta)) != 301 || !strings.HasSuffix(got.Meta, "…") {
		t.Fatalf("plain meta was not bounded: %d runes", len([]rune(got.Meta)))
	}
}

func TestMatchingEventIndexUsesIDThenPhase(t *testing.T) {
	events := []model.MessageEvent{
		{ID: "started-id", Kind: "tool", Phase: "started", CallKey: "same"},
		{ID: "completed-id", Kind: "tool", Phase: "completed", CallKey: "same"},
	}
	if got := matchingEventIndex(events, model.MessageEvent{ID: "completed-id", Kind: "tool", Phase: "started", CallKey: "same"}); got != 1 {
		t.Fatalf("ID match index = %d", got)
	}
	if got := matchingEventIndex(events, model.MessageEvent{Kind: "tool", Phase: "completed", CallKey: "same"}); got != 1 {
		t.Fatalf("phase match index = %d", got)
	}
}

func TestCompactToolNamesAcceptsTypedStringSlice(t *testing.T) {
	got, ok := compactToolNames([]string{"first", "second"}, 10)
	if !ok || len(got) != 2 || got[1] != "second" {
		t.Fatalf("typed tool names = %#v, %v", got, ok)
	}
}
