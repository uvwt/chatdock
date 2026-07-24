package httpapi

import (
	"context"
	"errors"
	"testing"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

func TestCompleteModelWithFallbackRetriesBeforeModelActivity(t *testing.T) {
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	primaryErr := errors.New("primary unavailable")
	var attempts []string
	var fallbackEvent map[string]any

	answer, usedCfg, err := completeModelWithFallback(
		context.Background(),
		primary,
		&fallback,
		func(event string, value any) error {
			if event == "model_fallback" {
				fallbackEvent, _ = value.(map[string]any)
			}
			return nil
		},
		func(cfg model.ModelConfig, emit func(string, any) error, markStarted func()) (string, error) {
			attempts = append(attempts, cfg.Model)
			if cfg.ProviderID == primary.ProviderID {
				return "", primaryErr
			}
			if emit == nil {
				t.Fatal("streaming fallback should retain the caller emitter")
			}
			return "backup answer", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "backup answer" || usedCfg.ProviderID != fallback.ProviderID || usedCfg.Model != fallback.Model {
		t.Fatalf("unexpected fallback result: answer=%q config=%#v", answer, usedCfg)
	}
	if len(attempts) != 2 || attempts[0] != primary.Model || attempts[1] != fallback.Model {
		t.Fatalf("unexpected attempts: %#v", attempts)
	}
	if fallbackEvent["from_model"] != primary.Model || fallbackEvent["to_model"] != fallback.Model {
		t.Fatalf("fallback event missing route details: %#v", fallbackEvent)
	}
}

func TestCompleteModelWithFallbackDoesNotRetryAfterVisibleOutput(t *testing.T) {
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	attempts := 0

	answer, usedCfg, err := completeModelWithFallback(
		context.Background(),
		primary,
		&fallback,
		func(string, any) error { return nil },
		func(cfg model.ModelConfig, emit func(string, any) error, markStarted func()) (string, error) {
			attempts++
			if emit == nil {
				t.Fatal("expected streaming emitter")
			}
			if err := emit("delta", llm.StreamDelta{Content: "partial"}); err != nil {
				return "", err
			}
			return "partial", errors.New("stream interrupted")
		},
	)
	if err == nil || answer != "partial" {
		t.Fatalf("expected primary partial failure, answer=%q err=%v", answer, err)
	}
	if attempts != 1 || usedCfg.ProviderID != primary.ProviderID {
		t.Fatalf("fallback must not run after visible output: attempts=%d config=%#v", attempts, usedCfg)
	}
}

func TestCompleteModelWithFallbackDoesNotRetryAfterBlockingToolCall(t *testing.T) {
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	attempts := 0

	_, usedCfg, err := completeModelWithFallback(
		context.Background(),
		primary,
		&fallback,
		nil,
		func(cfg model.ModelConfig, emit func(string, any) error, markStarted func()) (string, error) {
			attempts++
			if emit != nil {
				t.Fatal("blocking completion must keep a nil emitter")
			}
			markStarted()
			return "", errors.New("failed after tool side effect")
		},
	)
	if err == nil {
		t.Fatal("expected primary failure")
	}
	if attempts != 1 || usedCfg.ProviderID != primary.ProviderID {
		t.Fatalf("fallback must not run after a blocking tool call: attempts=%d config=%#v", attempts, usedCfg)
	}
}

func TestCompleteModelWithFallbackDoesNotRetryCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	attempts := 0

	_, _, err := completeModelWithFallback(
		ctx,
		primary,
		&fallback,
		nil,
		func(model.ModelConfig, func(string, any) error, func()) (string, error) {
			attempts++
			return "", context.Canceled
		},
	)
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled request must not fall back: attempts=%d err=%v", attempts, err)
	}
}
