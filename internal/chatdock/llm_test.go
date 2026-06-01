package chatdock

import "testing"

func TestStripThinkingContent(t *testing.T) {
	got := StripThinkingContent("hello <think>secret</think> world")
	if got != "hello  world" {
		t.Fatalf("unexpected stripped content: %q", got)
	}
	got = StripThinkingContent("hello <think>unfinished")
	if got != "hello" {
		t.Fatalf("unexpected unfinished think content: %q", got)
	}
}

func TestThinkingFilterAcrossChunks(t *testing.T) {
	f := NewThinkingFilter(true)
	parts := []string{"hel", "lo <thi", "nk>hide", "</think> ok"}
	var got string
	for _, p := range parts {
		got += f.Push(p)
	}
	got += f.Flush()
	if got != "hello  ok" {
		t.Fatalf("unexpected stream filter output: %q", got)
	}
}

func TestBuildChatMessagesLimitsContext(t *testing.T) {
	cfg := ModelConfig{SystemPrompt: "sys", MaxContextMessages: 2}
	history := []Message{{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}, {Role: "tool", Content: "ignored"}, {Role: "user", Content: "c"}}
	got := BuildChatMessages(cfg, history)
	if len(got) != 2 {
		t.Fatalf("expected system plus last valid message, got %#v", got)
	}
	if got[0]["role"] != "system" || got[1]["content"] != "c" {
		t.Fatalf("unexpected messages: %#v", got)
	}
}
