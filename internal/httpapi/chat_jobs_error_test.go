package httpapi

import (
	"strings"
	"testing"

	storepkg "chatdock/internal/store"
)

func TestNewMessageErrorKeepsFriendlyAndRawMessages(t *testing.T) {
	raw := `model api failed: {"error":{"message":"upstream unavailable"}}`
	got := newMessageError(" req_error_detail ", raw)

	if got.Message != "模型调用失败：上游模型服务返回错误。" {
		t.Fatalf("unexpected public message: %q", got.Message)
	}
	if got.Raw != raw {
		t.Fatalf("raw error changed: got=%q want=%q", got.Raw, raw)
	}
	if got.Code != "CHAT_STREAM_FAILED" || got.RequestID != "req_error_detail" || !got.Retryable {
		t.Fatalf("unexpected error metadata: %#v", got)
	}
}

func TestChatStreamErrorPayloadIncludesRawDetails(t *testing.T) {
	raw := "dial tcp: connection refused"
	payload := chatStreamErrorPayload(storepkg.ChatJob{RequestID: "req_stream_error"}, raw)

	if payload["message"] != "模型响应中断：无法连接上游模型服务。" {
		t.Fatalf("unexpected public stream error: %#v", payload)
	}
	if payload["raw"] != raw || payload["code"] != "UPSTREAM_UNAVAILABLE" {
		t.Fatalf("raw stream error details missing: %#v", payload)
	}
	if payload["request_id"] != "req_stream_error" || payload["retryable"] != true {
		t.Fatalf("unexpected stream error metadata: %#v", payload)
	}
}

func TestNewMessageErrorPrioritizesContextOverflowOverTimeoutWrapper(t *testing.T) {
	raw := `model api failed: 504 Gateway Timeout: upstream wrapper [context_too_large]`
	got := newMessageError("req_context_timeout", raw)

	if got.Message != "模型调用失败：上下文过长，请减少当前对话或工具输出后重试。" {
		t.Fatalf("unexpected public message: %q", got.Message)
	}
	if got.Code != "UPSTREAM_CONTEXT_TOO_LARGE" || got.Retryable {
		t.Fatalf("context overflow must outrank timeout and stay non-retryable: %#v", got)
	}
}

func TestNewMessageErrorMarksBadRequestAsNotRetryable(t *testing.T) {
	raw := `model api failed: 400 Bad Request: {"error":{"code":"InvalidParameter","message":"Invalid request body"}}`
	got := newMessageError("req_bad_request", raw)

	if got.Message != "模型调用失败：请求参数不受当前模型供应商支持。" {
		t.Fatalf("unexpected public message: %q", got.Message)
	}
	if got.Code != "UPSTREAM_BAD_REQUEST" || got.Retryable {
		t.Fatalf("bad request must not be marked retryable: %#v", got)
	}
}

func TestChatErrorClassifiesContextTooLargeSeparately(t *testing.T) {
	raw := `model api failed: 400 Bad Request: {"error":{"message":"Your input exceeds the context window of this model.","code":"context_too_large"}}`
	messageError := newMessageError("req-context", raw)
	if messageError.Code != "UPSTREAM_CONTEXT_TOO_LARGE" {
		t.Fatalf("code = %q, want UPSTREAM_CONTEXT_TOO_LARGE", messageError.Code)
	}
	if messageError.Retryable {
		t.Fatal("context overflow must not be user-retryable without changing the input")
	}
	if !strings.Contains(messageError.Message, "上下文过长") {
		t.Fatalf("unexpected public message: %q", messageError.Message)
	}
}
