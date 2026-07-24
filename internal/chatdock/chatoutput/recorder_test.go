package chatoutput

import (
	"testing"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
	"chatdock/internal/chatdock/store"
)

func TestAssistantOutputRecorderFlushesFirstReasoningAndContentDeltasImmediately(t *testing.T) {
	store, err := store.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.SaveModelConfig(model.ModelConfig{BaseURL: "http://127.0.0.1:1", Model: "demo"}); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	job, _, _, _, err := store.PrepareChatJob(model.ChatRequest{SessionID: session.ID, Message: "hello"}, "req_first_delta")
	if err != nil {
		t.Fatal(err)
	}

	recorder := NewRecorder(store, session.ID, job.ID)
	if err := recorder.Emit("delta", llm.StreamDelta{ReasoningContent: "r"}); err != nil {
		t.Fatal(err)
	}
	_, events, err := store.ChatJobEventsAfter(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "delta" {
		t.Fatalf("first reasoning delta should flush immediately: %#v", events)
	}

	if err := recorder.Emit("delta", llm.StreamDelta{Content: "O"}); err != nil {
		t.Fatal(err)
	}
	_, events, err = store.ChatJobEventsAfter(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Event != "delta" {
		t.Fatalf("first content delta should flush independently of reasoning: %#v", events)
	}
}
