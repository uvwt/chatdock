package httpapi

import (
	"errors"
	"net/http"
	"testing"

	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

func TestChatPreparationHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "session missing", err: model.ErrSessionNotFound, want: http.StatusNotFound},
		{name: "invalid request", err: storepkg.ErrInvalidChatRequest, want: http.StatusBadRequest},
		{name: "invalid request wrapped", err: errors.Join(errors.New("context"), storepkg.ErrInvalidChatRequest), want: http.StatusBadRequest},
		{name: "shutting down", err: errAppShuttingDown, want: http.StatusServiceUnavailable},
		{name: "database failure", err: errors.New("database is locked"), want: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := chatPreparationHTTPStatus(test.err); got != test.want {
				t.Fatalf("chatPreparationHTTPStatus(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}
