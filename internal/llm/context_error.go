package llm

import (
	"errors"
	"strings"
)

var contextTooLargeFragments = []string{
	"context_too_large",
	"context_length_exceeded",
	"context length exceeded",
	"maximum context length",
	"exceeds the context window",
	"prompt is too long",
	"reduce the length of the messages",
}

func IsContextTooLargeErrorText(raw string) bool {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return false
	}
	for _, fragment := range contextTooLargeFragments {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func IsContextTooLargeModelError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *modelAPIResponseError
	if errors.As(err, &apiErr) && apiErr.contextTooLarge {
		return true
	}
	return IsContextTooLargeErrorText(err.Error())
}
