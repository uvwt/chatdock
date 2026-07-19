package llm

import (
	"fmt"
	"io"
	"net/http"
)

const maxModelErrorResponseBytes int64 = 256 << 10

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
	return fmt.Errorf("%s: %s: %s", prefix, resp.Status, summarizeModelProviderBody(resp.Header.Get("Content-Type"), body))
}
