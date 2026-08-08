package httpapi

import (
	"errors"
	"net/http"

	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
)

var errAppShuttingDown = errors.New("ChatDock is shutting down")

func chatPreparationHTTPStatus(err error) int {
	switch {
	case errors.Is(err, model.ErrSessionNotFound):
		return http.StatusNotFound
	case errors.Is(err, storepkg.ErrInvalidChatRequest):
		return http.StatusBadRequest
	case errors.Is(err, errAppShuttingDown):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
