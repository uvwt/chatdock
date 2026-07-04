package chatdock

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type requestContextKey struct{}

type logFields map[string]any

func newRequestID() string {
	return "req_" + model.NewID()
}

func withRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = newRequestID()
	}
	return context.WithValue(ctx, requestContextKey{}, requestID)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(requestContextKey{}).(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func requestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id := requestIDFromContext(r.Context()); id != "" {
		return id
	}
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}

func logInfo(event string, fields logFields) {
	writeStructuredLog("info", event, fields)
}

func logError(event string, err error, fields logFields) {
	if fields == nil {
		fields = logFields{}
	}
	if err != nil {
		fields["error"] = sanitizeLogText(err.Error(), 4000)
	}
	writeStructuredLog("error", event, fields)
}

func writeStructuredLog(level string, event string, fields logFields) {
	if fields == nil {
		fields = logFields{}
	}
	entry := map[string]any{
		"level": strings.TrimSpace(level),
		"time":  time.Now().Format(time.RFC3339Nano),
		"event": strings.TrimSpace(event),
	}
	for key, value := range fields {
		if strings.TrimSpace(key) == "" {
			continue
		}
		entry[key] = value
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		log.Printf(`{"level":"error","event":"log_marshal_failed","error":%q}`, err.Error())
		return
	}
	log.Print(string(raw))
}

func sanitizeLogText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
