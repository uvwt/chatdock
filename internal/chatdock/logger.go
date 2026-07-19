package chatdock

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
)

type requestContextKey struct{}

type logFields map[string]any

var (
	logAuthorizationPattern = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^\s,;]+`)
	logBearerPattern        = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	logSecretFieldPattern   = regexp.MustCompile(`(?i)((?:"|')?(?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|credential|password)(?:"|')?\s*[:=]\s*(?:"|')?)[^"',\s}\]]+`)
	logURLUserinfoPattern   = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
)

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
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		entry[key] = sanitizeLogValue(key, value)
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		log.Printf(`{"level":"error","event":"log_marshal_failed","error":%q}`, err.Error())
		return
	}
	log.Print(string(raw))
}

func sanitizeLogText(value string, limit int) string {
	value = redactSensitiveLogText(strings.TrimSpace(value))
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func sanitizeLogValue(key string, value any) any {
	if isSensitiveLogKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case string:
		return sanitizeLogText(typed, 4000)
	case error:
		return sanitizeLogText(typed.Error(), 4000)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for nestedKey, nestedValue := range typed {
			out[nestedKey] = sanitizeLogValue(nestedKey, nestedValue)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sanitizeLogValue("", item)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		for i, item := range typed {
			out[i] = sanitizeLogText(item, 4000)
		}
		return out
	default:
		return typed
	}
}

func isSensitiveLogKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.NewReplacer("-", "_", " ", "_").Replace(key)
	switch key {
	case "authorization", "api_key", "apikey", "access_token", "refresh_token", "token", "credential", "password", "secret":
		return true
	default:
		return false
	}
}

func redactSensitiveLogText(value string) string {
	value = logAuthorizationPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = logBearerPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = logSecretFieldPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	return logURLUserinfoPattern.ReplaceAllString(value, `${1}[REDACTED]@`)
}
