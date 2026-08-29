package httpapi

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

func TestCompleteModelWithFallbackRetriesBeforeModelActivity(t *testing.T) {
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	var attempts []string
	var retryEvents []map[string]any
	var fallbackEvent map[string]any

	answer, usedCfg, err := completeModelWithFallbackRetryDelays(
		context.Background(),
		primary,
		&fallback,
		func(event string, value any) error {
			if event == "model_retry" {
				data, _ := value.(map[string]any)
				retryEvents = append(retryEvents, data)
			}
			if event == "model_fallback" {
				fallbackEvent, _ = value.(map[string]any)
			}
			return nil
		},
		[]time.Duration{0, 0},
		func(cfg model.ModelConfig, emit func(string, any) error, markStarted func()) (string, error) {
			attempts = append(attempts, cfg.Model)
			if cfg.ProviderID == primary.ProviderID {
				return "", io.ErrUnexpectedEOF
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
	if len(attempts) != 4 || attempts[0] != primary.Model || attempts[1] != primary.Model || attempts[2] != primary.Model || attempts[3] != fallback.Model {
		t.Fatalf("unexpected attempts: %#v", attempts)
	}
	if len(retryEvents) != 2 || retryEvents[0]["attempt"] != 1 || retryEvents[1]["attempt"] != 2 {
		t.Fatalf("unexpected retry events: %#v", retryEvents)
	}
	if fallbackEvent["from_model"] != primary.Model || fallbackEvent["to_model"] != fallback.Model {
		t.Fatalf("fallback event missing route details: %#v", fallbackEvent)
	}
}

func TestCompleteModelWithFallbackKeepsPrimaryWhenRetryRecovers(t *testing.T) {
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	attempts := 0
	retryEvents := 0
	fallbackEvents := 0

	answer, usedCfg, err := completeModelWithFallbackRetryDelays(
		context.Background(),
		primary,
		&fallback,
		func(event string, value any) error {
			switch event {
			case "model_retry":
				retryEvents++
			case "model_fallback":
				fallbackEvents++
			}
			return nil
		},
		[]time.Duration{0, 0},
		func(cfg model.ModelConfig, emit func(string, any) error, markStarted func()) (string, error) {
			if cfg.ProviderID != primary.ProviderID {
				t.Fatal("fallback must not run after the primary model recovers")
			}
			attempts++
			if attempts < 3 {
				return "", io.ErrUnexpectedEOF
			}
			return "primary answer", nil
		},
	)
	if err != nil || answer != "primary answer" || usedCfg.ProviderID != primary.ProviderID {
		t.Fatalf("unexpected recovered result: answer=%q config=%#v err=%v", answer, usedCfg, err)
	}
	if attempts != 3 || retryEvents != 2 || fallbackEvents != 0 {
		t.Fatalf("unexpected recovery attempts=%d retry_events=%d fallback_events=%d", attempts, retryEvents, fallbackEvents)
	}
}

func TestCompleteModelWithFallbackDoesNotRetryDeterministicFailure(t *testing.T) {
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	attempts := 0

	_, usedCfg, err := completeModelWithFallbackRetryDelays(
		context.Background(),
		primary,
		&fallback,
		nil,
		[]time.Duration{0, 0},
		func(cfg model.ModelConfig, emit func(string, any) error, markStarted func()) (string, error) {
			attempts++
			return "", errors.New("expected JSON object for tool arguments")
		},
	)
	if err == nil || attempts != 1 || usedCfg.ProviderID != primary.ProviderID {
		t.Fatalf("deterministic failure must return immediately: attempts=%d config=%#v err=%v", attempts, usedCfg, err)
	}
}

func TestCompleteModelWithFallbackRoutesContextOverflowWithoutSameModelRetry(t *testing.T) {
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	attempts := make([]string, 0, 2)
	retryEvents := 0
	fallbackEvents := 0

	answer, usedCfg, err := completeModelWithFallbackRetryDelays(
		context.Background(),
		primary,
		&fallback,
		func(event string, value any) error {
			switch event {
			case "model_retry":
				retryEvents++
			case "model_fallback":
				fallbackEvents++
			}
			return nil
		},
		[]time.Duration{0, 0},
		func(cfg model.ModelConfig, emit func(string, any) error, markStarted func()) (string, error) {
			attempts = append(attempts, cfg.ProviderID)
			if cfg.ProviderID == primary.ProviderID {
				return "", errors.New("model api failed: 503 Service Unavailable: gateway [context_too_large]")
			}
			return "fallback answer", nil
		},
	)
	if err != nil || answer != "fallback answer" || usedCfg.ProviderID != fallback.ProviderID {
		t.Fatalf("unexpected fallback result: answer=%q config=%#v err=%v", answer, usedCfg, err)
	}
	if len(attempts) != 2 || attempts[0] != primary.ProviderID || attempts[1] != fallback.ProviderID {
		t.Fatalf("context overflow should go directly to fallback: %#v", attempts)
	}
	if retryEvents != 0 || fallbackEvents != 1 {
		t.Fatalf("unexpected retry/fallback events: retries=%d fallbacks=%d", retryEvents, fallbackEvents)
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

func TestCompleteModelWithFallbackStopsDuringRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	primary := model.ModelConfig{ProviderID: "primary", Model: "primary-model"}
	fallback := model.ModelConfig{ProviderID: "backup", Model: "backup-model"}
	attempts := 0

	_, usedCfg, err := completeModelWithFallbackRetryDelays(
		ctx,
		primary,
		&fallback,
		func(event string, value any) error {
			if event == "model_retry" {
				cancel()
			}
			return nil
		},
		[]time.Duration{time.Hour, time.Hour},
		func(model.ModelConfig, func(string, any) error, func()) (string, error) {
			attempts++
			return "", io.ErrUnexpectedEOF
		},
	)
	if !errors.Is(err, context.Canceled) || attempts != 1 || usedCfg.ProviderID != primary.ProviderID {
		t.Fatalf("retry wait must stop with the request context: attempts=%d config=%#v err=%v", attempts, usedCfg, err)
	}
}
