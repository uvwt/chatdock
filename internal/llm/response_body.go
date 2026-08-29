package llm

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxModelErrorResponseBytes int64 = 256 << 10

type modelAPIResponseError struct {
	prefix          string
	status          string
	statusCode      int
	summary         string
	retryAfter      time.Duration
	contextTooLarge bool
}

func (e *modelAPIResponseError) Error() string {
	message := fmt.Sprintf("%s: %s: %s", e.prefix, e.status, e.summary)
	if e.contextTooLarge && !IsContextTooLargeErrorText(e.summary) {
		message += " [context_too_large]"
	}
	return message
}

func modelResponseBodyLimit(resp *http.Response, successLimit int64) int64 {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return maxModelErrorResponseBytes
	}
	return successLimit
}

func readModelResponseBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("model response limit must be positive")
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("model response exceeds %d bytes", maxBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read model response: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("model response exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

func modelAPIError(prefix string, resp *http.Response, body []byte) error {
	return &modelAPIResponseError{
		prefix:          prefix,
		status:          resp.Status,
		statusCode:      resp.StatusCode,
		summary:         summarizeModelProviderBody(resp.Header.Get("Content-Type"), body),
		retryAfter:      parseModelRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		contextTooLarge: IsContextTooLargeErrorText(string(body)),
	}
}

func modelStreamError(prefix string, body []byte) error {
	return &modelAPIResponseError{
		prefix:          prefix,
		status:          "stream error",
		summary:         summarizeModelProviderBody("application/json", body),
		contextTooLarge: IsContextTooLargeErrorText(string(body)),
	}
}
