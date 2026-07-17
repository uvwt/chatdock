package chatdock

import (
	"testing"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

func TestAssistantOutputRecorderFlushesFirstReasoningAndContentDeltasImmediately(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	if _, err := app.store.SaveModelConfig("default", model.ModelConfig{BaseURL: "http://127.0.0.1:1", Model: "demo"}); err != nil {
		t.Fatal(err)
	}
	session, err := app.store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	job, _, _, _, err := app.store.PrepareChatJob("default", model.ChatRequest{SessionID: session.ID, Message: "hello"}, "req_first_delta")
	if err != nil {
		t.Fatal(err)
	}

	recorder := newAssistantOutputRecorder(app, "default", session.ID, job.ID)
	if err := recorder.emit("delta", llm.StreamDelta{ReasoningContent: "r"}); err != nil {
		t.Fatal(err)
	}
	_, events, err := app.store.ChatJobEventsAfter("default", job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "delta" {
		t.Fatalf("first reasoning delta should flush immediately: %#v", events)
	}

	if err := recorder.emit("delta", llm.StreamDelta{Content: "O"}); err != nil {
		t.Fatal(err)
	}
	_, events, err = app.store.ChatJobEventsAfter("default", job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Event != "delta" {
		t.Fatalf("first content delta should flush independently of reasoning: %#v", events)
	}
}
