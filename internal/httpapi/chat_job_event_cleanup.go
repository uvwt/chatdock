package httpapi

import (
	"context"
	"time"
)

const (
	chatJobEventRetention       = 24 * time.Hour
	chatJobEventCleanupInterval = time.Hour
)

func (a *Server) runChatJobEventCleanup(ctx context.Context) {
	a.pruneExpiredChatJobEvents()
	ticker := time.NewTicker(chatJobEventCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.pruneExpiredChatJobEvents()
		}
	}
}

func (a *Server) pruneExpiredChatJobEvents() {
	cutoff := time.Now().Add(-chatJobEventRetention)
	deleted, err := a.store.PruneChatJobStreamingEventsBefore(cutoff)
	if err != nil {
		logError("chat_job_event_cleanup_failed", err, nil)
		return
	}
	if deleted > 0 {
		logInfo("chat_job_event_cleanup_finished", logFields{"deleted": deleted})
	}
}
