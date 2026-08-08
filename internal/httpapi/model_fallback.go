package httpapi

import (
	"context"
	"strings"
	"time"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

var defaultModelRetryDelays = [...]time.Duration{500 * time.Millisecond, time.Second}

type modelAttemptTracker struct {
	emit    func(string, any) error
	started bool
}

func (t *modelAttemptTracker) markStarted() {
	t.started = true
}

func (t *modelAttemptTracker) callback() func(string, any) error {
	if t.emit == nil {
		return nil
	}
	return t.forward
}

func (a *Server) resolveFallbackModelConfig(ctx context.Context, sessionID string, cfg model.ModelConfig) *model.ModelConfig {
	fallbackCfg, err := a.store.ResolveFallbackModelConfig(cfg)
	if err == nil {
		return fallbackCfg
	}
	logError("chat_fallback_model_resolve_failed", err, logFields{
		"request_id":  requestIDFromContext(ctx),
		"session_id":  sessionID,
		"provider_id": cfg.ProviderID,
		"model":       cfg.Model,
	})
	return nil
}

func (t *modelAttemptTracker) forward(event string, value any) error {
	// 一旦模型已经输出正文、思考或开始执行工具，就不能重放到备用模型，
	// 否则会造成重复文本或重复副作用。标记必须发生在下游事件落库之前。
	if event == "tool_call_start" {
		t.started = true
	}
	if event == "delta" {
		if delta, ok := value.(llm.StreamDelta); ok && !delta.Empty() {
			t.started = true
		}
	}
	if t.emit == nil {
		return nil
	}
	return t.emit(event, value)
}

func completeModelWithFallback(
	ctx context.Context,
	primary model.ModelConfig,
	fallback *model.ModelConfig,
	emit func(string, any) error,
	complete func(model.ModelConfig, func(string, any) error, func()) (string, error),
) (string, model.ModelConfig, error) {
	return completeModelWithFallbackRetryDelays(ctx, primary, fallback, emit, defaultModelRetryDelays[:], complete)
}

func completeModelWithFallbackRetryDelays(
	ctx context.Context,
	primary model.ModelConfig,
	fallback *model.ModelConfig,
	emit func(string, any) error,
	retryDelays []time.Duration,
	complete func(model.ModelConfig, func(string, any) error, func()) (string, error),
) (string, model.ModelConfig, error) {
	answer, primaryAttempt, err := completeModelAttempt(ctx, primary, emit, retryDelays, complete)
	retryable := llm.IsRetryableModelError(err)
	if err == nil || fallback == nil || primaryAttempt.started || ctx.Err() != nil || isClientCanceled(ctx, err) || !retryable {
		return answer, primary, err
	}

	payload := map[string]any{
		"from_provider_id": primary.ProviderID,
		"from_model":       primary.Model,
		"to_provider_id":   fallback.ProviderID,
		"to_model":         fallback.Model,
		"reason":           publicChatErrorMessage(err.Error()),
	}
	if emit != nil {
		if emitErr := emit("model_fallback", payload); emitErr != nil {
			return answer, primary, emitErr
		}
	}
	logInfo("chat_model_fallback_started", logFields{
		"request_id":       requestIDFromContext(ctx),
		"from_provider_id": primary.ProviderID,
		"from_model":       primary.Model,
		"to_provider_id":   fallback.ProviderID,
		"to_model":         fallback.Model,
		"reason":           strings.TrimSpace(err.Error()),
	})

	answer, _, err = completeModelAttempt(ctx, *fallback, emit, retryDelays, complete)
	return answer, *fallback, err
}

func completeModelAttempt(
	ctx context.Context,
	cfg model.ModelConfig,
	emit func(string, any) error,
	retryDelays []time.Duration,
	complete func(model.ModelConfig, func(string, any) error, func()) (string, error),
) (string, *modelAttemptTracker, error) {
	attempt := &modelAttemptTracker{emit: emit}
	answer, err := complete(cfg, attempt.callback(), attempt.markStarted)
	for retryIndex, fallbackDelay := range retryDelays {
		if err == nil || attempt.started || ctx.Err() != nil || isClientCanceled(ctx, err) {
			return answer, attempt, err
		}

		delay, retryable := llm.ModelRetryDelay(err, fallbackDelay)
		if !retryable {
			return answer, attempt, err
		}

		payload := map[string]any{
			"provider_id": cfg.ProviderID,
			"model":       cfg.Model,
			"attempt":     retryIndex + 1,
			"max_retries": len(retryDelays),
			"delay_ms":    delay.Milliseconds(),
			"reason":      publicChatErrorMessage(err.Error()),
		}
		if emit != nil {
			if emitErr := emit("model_retry", payload); emitErr != nil {
				return answer, attempt, emitErr
			}
		}
		logInfo("chat_model_retry_scheduled", logFields{
			"request_id":  requestIDFromContext(ctx),
			"provider_id": cfg.ProviderID,
			"model":       cfg.Model,
			"attempt":     retryIndex + 1,
			"max_retries": len(retryDelays),
			"delay_ms":    delay.Milliseconds(),
			"reason":      strings.TrimSpace(err.Error()),
		})

		if waitErr := waitForModelRetry(ctx, delay); waitErr != nil {
			return answer, attempt, waitErr
		}
		answer, err = complete(cfg, attempt.callback(), attempt.markStarted)
	}
	return answer, attempt, err
}

func waitForModelRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
