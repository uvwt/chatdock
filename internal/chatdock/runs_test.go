package chatdock

import (
	"errors"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestToolRunEmitterReturnsRunEventErrors(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	emitErr := errors.New("persist frontend run event")
	recorder := &activeToolRun{LastArgs: map[string]any{}, StartedAt: map[string]time.Time{}}
	emit := app.toolRunEmitter("default", "session-a", recorder, func(event string, value any) error {
		if event == "run_event" {
			return emitErr
		}
		return nil
	})
	err = emit("tool_call_start", map[string]any{"tool": "demo_tool", "arguments": map[string]any{"value": 1}})
	if !errors.Is(err, emitErr) {
		t.Fatalf("expected run event error, got %v", err)
	}
	if !recorder.Created || recorder.RunID == "" {
		t.Fatalf("tool run should be created before emitting its event: %#v", recorder)
	}
}

func TestFinishRecordedToolRunReturnsStoreAndEmitErrors(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	run, err := app.store.StartMCPRun("default", "session-a", "test run")
	if err != nil {
		t.Fatal(err)
	}
	emitErr := errors.New("persist run finish")
	err = app.finishRecordedToolRun(&activeToolRun{RunID: run.ID, Created: true}, nil, func(event string, value any) error {
		if event != "run_finish" {
			t.Fatalf("unexpected finish event %q", event)
		}
		return emitErr
	})
	if !errors.Is(err, emitErr) {
		t.Fatalf("expected finish emit error, got %v", err)
	}

	err = app.finishRecordedToolRun(&activeToolRun{RunID: "missing", Created: true}, nil, nil)
	if err == nil {
		t.Fatal("expected missing run to return a store error")
	}
}
