package chatdock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"chatdock/internal/chatdock/model"
)

func compactSessionToolEventDetails(session *model.Session) *model.Session {
	if session == nil {
		return nil
	}
	out := *session
	out.Messages = make([]model.Message, len(session.Messages))
	for messageIndex, message := range session.Messages {
		out.Messages[messageIndex] = message
		if len(message.Events) > 0 {
			out.Messages[messageIndex].Events = make([]model.MessageEvent, len(message.Events))
			for eventIndex, event := range message.Events {
				out.Messages[messageIndex].Events[eventIndex] = compactMessageEvent(session.ID, message.ID, messageIndex, eventIndex, -1, event)
			}
		}
		if len(message.Parts) > 0 {
			out.Messages[messageIndex].Parts = make([]model.MessagePart, len(message.Parts))
			for partIndex, part := range message.Parts {
				out.Messages[messageIndex].Parts[partIndex] = part
				if part.Event != nil {
					eventIndex := matchingEventIndex(message.Events, *part.Event)
					event := compactMessageEvent(session.ID, message.ID, messageIndex, eventIndex, partIndex, *part.Event)
					out.Messages[messageIndex].Parts[partIndex].Event = &event
				}
			}
		}
	}
	return &out
}

func compactMessageEvent(sessionID string, messageID string, messageIndex int, eventIndex int, partIndex int, event model.MessageEvent) model.MessageEvent {
	event.ID = stableToolEventID(sessionID, messageID, messageIndex, eventIndex, partIndex, event)
	details := map[string]any{}
	if len(event.Details) > 0 {
		details = compactToolEventDetails(event.Details)
	}
	if meta := strings.TrimSpace(event.Meta); meta != "" {
		decoded := map[string]any{}
		if err := json.Unmarshal([]byte(meta), &decoded); err == nil {
			if len(event.Details) == 0 {
				details = compactToolEventDetails(decoded)
			}
			event.Meta = ""
		} else {
			event.Meta = truncateRunes(meta, 300)
		}
	}
	details["lazy"] = true
	details["session_id"] = sessionID
	details["event_id"] = event.ID
	details["message_index"] = messageIndex
	if eventIndex >= 0 {
		details["event_index"] = eventIndex
	}
	if partIndex >= 0 {
		details["part_index"] = partIndex
	}
	event.Details = details
	return event
}

func compactToolEventDetails(details map[string]any) map[string]any {
	out := map[string]any{}
	copyCompactValue(out, details, "event")
	copyCompactValue(out, details, "tool")
	copyCompactValue(out, details, "ok")
	copyCompactValue(out, details, "duration_ms")
	copyCompactValue(out, details, "tool_count")
	copyCompactValue(out, details, "builtin_tool_count")
	copyCompactValue(out, details, "server")
	copyCompactValue(out, details, "action")
	if value := shortString(details["error"], 800); value != "" {
		out["error"] = value
	}
	if args := compactToolArguments(details["arguments"]); len(args) > 0 {
		out["arguments"] = args
	}
	if data, ok := details["data"].(map[string]any); ok {
		out["data"] = compactToolEventData(data)
	}
	return out
}

func compactToolEventData(data map[string]any) map[string]any {
	out := map[string]any{}
	copyCompactValue(out, data, "event")
	copyCompactValue(out, data, "tool")
	copyCompactValue(out, data, "ok")
	copyCompactValue(out, data, "duration_ms")
	copyCompactValue(out, data, "tool_count")
	copyCompactValue(out, data, "builtin_tool_count")
	copyCompactValue(out, data, "server")
	copyCompactValue(out, data, "action")
	copyCompactValue(out, data, "count")
	copyCompactValue(out, data, "query")
	if value := shortString(data["error"], 800); value != "" {
		out["error"] = value
	}
	if args := compactToolArguments(data["arguments"]); len(args) > 0 {
		out["arguments"] = args
	}
	if result := compactToolResult(data["result"]); len(result) > 0 {
		out["result"] = result
	}
	return out
}

func compactToolArguments(value any) map[string]any {
	args, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	copyCompactValue(out, args, "name")
	copyCompactValue(out, args, "query")
	copyCompactValue(out, args, "tool")
	if names, ok := compactStringSlice(args["names"], 20); ok {
		out["names"] = names
	}
	if nested, ok := args["arguments"].(map[string]any); ok {
		nestedOut := map[string]any{}
		copyCompactValue(nestedOut, nested, "query")
		copyCompactValue(nestedOut, nested, "url")
		copyCompactValue(nestedOut, nested, "path")
		if len(nestedOut) > 0 {
			out["arguments"] = nestedOut
		}
	}
	return out
}

func compactToolResult(value any) map[string]any {
	result, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	copyCompactValue(out, result, "tool")
	copyCompactValue(out, result, "query")
	copyCompactValue(out, result, "count")
	copyCompactValue(out, result, "status")
	if tools, ok := compactToolNames(result["tools"], 20); ok {
		out["tools"] = tools
	}
	return out
}

func compactToolNames(value any, limit int) ([]string, bool) {
	items := []any{}
	switch typed := value.(type) {
	case []any:
		items = typed
	case []string:
		items = make([]any, len(typed))
		for i, item := range typed {
			items[i] = item
		}
	default:
		return nil, false
	}
	out := make([]string, 0, min(len(items), limit))
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		switch typed := item.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				out = append(out, typed)
			}
		case map[string]any:
			name := firstNonEmptyString(typed["name"], typed["full_name"], typed["title"])
			if name != "" {
				out = append(out, name)
			}
		}
	}
	return out, len(out) > 0
}

func compactStringSlice(value any, limit int) ([]string, bool) {
	items := []any{}
	switch typed := value.(type) {
	case []any:
		items = typed
	case []string:
		items = make([]any, len(typed))
		for i, item := range typed {
			items[i] = item
		}
	default:
		return nil, false
	}
	out := make([]string, 0, min(len(items), limit))
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" {
			out = append(out, text)
		}
	}
	return out, len(out) > 0
}

func copyCompactValue(out map[string]any, source map[string]any, key string) {
	value, ok := source[key]
	if !ok || value == nil {
		return
	}
	switch typed := value.(type) {
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			out[key] = truncateRunes(trimmed, 300)
		}
	case bool:
		out[key] = typed
	case int, int32, int64, float32, float64, jsonNumber:
		out[key] = typed
	default:
		text := shortString(value, 300)
		if text != "" {
			out[key] = text
		}
	}
}

type jsonNumber interface{ String() string }

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
			return truncateRunes(text, 300)
		}
	}
	return ""
}

func shortString(value any, maxRunes int) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	return truncateRunes(text, maxRunes)
}

func truncateRunes(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}

func stableToolEventID(sessionID string, messageID string, messageIndex int, eventIndex int, partIndex int, event model.MessageEvent) string {
	if strings.TrimSpace(event.ID) != "" {
		return strings.TrimSpace(event.ID)
	}
	messageKey := strings.TrimSpace(messageID)
	if messageKey == "" {
		messageKey = fmt.Sprintf("message:%d", messageIndex)
	}
	eventKey := ""
	if eventIndex >= 0 {
		eventKey = fmt.Sprintf("event:%d", eventIndex)
	} else if partIndex >= 0 {
		eventKey = fmt.Sprintf("part:%d", partIndex)
	} else {
		eventKey = "event:unknown"
	}
	seed := strings.Join([]string{sessionID, messageKey, eventKey, event.Kind, event.Phase, event.CallKey, event.Text}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	return "evt_" + hex.EncodeToString(sum[:])[:24]
}

func matchingEventIndex(events []model.MessageEvent, target model.MessageEvent) int {
	if strings.TrimSpace(target.ID) != "" {
		for i, event := range events {
			if event.ID == target.ID {
				return i
			}
		}
	}
	for i, event := range events {
		if target.CallKey != "" && event.CallKey == target.CallKey && event.Kind == target.Kind && event.Phase == target.Phase {
			return i
		}
	}
	for i, event := range events {
		if event.Kind == target.Kind && event.Phase == target.Phase && event.Text == target.Text && event.Meta == target.Meta {
			return i
		}
	}
	return -1
}

func (a *App) handleGetSessionToolEventByID(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	eventID := r.PathValue("event_id")
	event, err := a.store.SessionMessageEventByID(sessionID, eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("tool event not found"))
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"event": event})
}

func (a *App) handleGetSessionToolEvent(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	messageIndex, err := parseRequiredIndex(r, "message_index")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	eventIndex := -1
	partIndex := -1
	if raw := strings.TrimSpace(r.URL.Query().Get("event_index")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("event index out of range"))
			return
		}
		eventIndex = parsed
	} else if raw := strings.TrimSpace(r.URL.Query().Get("part_index")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("part index out of range"))
			return
		}
		partIndex = parsed
	} else {
		writeError(w, http.StatusBadRequest, fmt.Errorf("event_index or part_index is required"))
		return
	}
	event, err := a.store.SessionMessageEventByIndex(sessionID, messageIndex, eventIndex, partIndex)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"event": event})
}

func parseRequiredIndex(r *http.Request, name string) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 {
		return 0, fmt.Errorf("%s is invalid", name)
	}
	return index, nil
}
