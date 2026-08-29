package llm

import (
	"strings"
	"testing"

	"chatdock/internal/model"
)

func TestModelStreamsRejectMalformedAndExplicitErrorChunks(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"malformed": {body: "data: {not-json}\n\n", want: "decode model stream chunk"},
		"error":     {body: `data: {"error":{"message":"额度不足"}}` + "\n\n", want: "model stream failed"},
	}
	for name, tc := range cases {
		t.Run("plain/"+name, func(t *testing.T) {
			_, err := readModelStream(strings.NewReader(tc.body), model.ModelConfig{}, func(StreamDelta) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("plain stream error = %v, want %q", err, tc.want)
			}
		})
		t.Run("tool/"+name, func(t *testing.T) {
			_, err := readModelToolStream(strings.NewReader(tc.body), model.ModelConfig{}, func(string, any) error { return nil })
			want := strings.Replace(tc.want, "model stream", "model tool stream", 1)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("tool stream error = %v, want %q", err, want)
			}
		})
	}
}

func TestModelStreamsStillIgnoreValidMetadataChunks(t *testing.T) {
	body := strings.NewReader(`data: {"usage":{"prompt_tokens":12}}` + "\n\ndata: [DONE]\n\n")
	answer, err := readModelStream(body, model.ModelConfig{}, func(StreamDelta) error { return nil })
	if err != nil || answer != "" {
		t.Fatalf("metadata stream answer=%q error=%v", answer, err)
	}

	toolBody := strings.NewReader(`data: {"usage":{"prompt_tokens":12}}` + "\n\ndata: [DONE]\n\n")
	result, err := readModelToolStream(toolBody, model.ModelConfig{}, func(string, any) error { return nil })
	if err != nil || result.Content != "" || len(result.ToolCalls) != 0 {
		t.Fatalf("metadata tool stream result=%#v error=%v", result, err)
	}
}

func TestModelStreamReturnsPartialContentAlongsideProtocolError(t *testing.T) {
	body := strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: {bad}\n\n")
	answer, err := readModelStream(body, model.ModelConfig{}, func(StreamDelta) error { return nil })
	if answer != "partial" || err == nil || !strings.Contains(err.Error(), "decode model stream chunk") {
		t.Fatalf("partial stream answer=%q error=%v", answer, err)
	}
}

func TestModelStreamsKeepContextOverflowClassificationBeyondSummaryLimit(t *testing.T) {
	body := `data: {"error":{"message":"` + strings.Repeat("gateway metadata ", 30) + `","code":"context_too_large"}}` + "\n\n"

	_, plainErr := readModelStream(strings.NewReader(body), model.ModelConfig{}, func(StreamDelta) error { return nil })
	if !IsContextTooLargeModelError(plainErr) || !strings.Contains(plainErr.Error(), "context_too_large") {
		t.Fatalf("plain stream lost context overflow classification: %v", plainErr)
	}

	_, toolErr := readModelToolStream(strings.NewReader(body), model.ModelConfig{}, func(string, any) error { return nil })
	if !IsContextTooLargeModelError(toolErr) || !strings.Contains(toolErr.Error(), "context_too_large") {
		t.Fatalf("tool stream lost context overflow classification: %v", toolErr)
	}
}

func TestModelStreamTransportErrorStaysRetryable(t *testing.T) {
	body := `data: {"error":{"message":"connection reset by peer"}}` + "\n\n"
	_, err := readModelToolStream(strings.NewReader(body), model.ModelConfig{}, func(string, any) error { return nil })
	if err == nil {
		t.Fatal("expected in-band transport error")
	}
	if !IsRetryableModelError(err) {
		t.Fatalf("in-band transport error must remain fallback eligible: %v", err)
	}
	if _, retry := ModelRetryDelay(err, 0); !retry {
		t.Fatalf("in-band transport error must remain same-model retryable: %v", err)
	}
}
