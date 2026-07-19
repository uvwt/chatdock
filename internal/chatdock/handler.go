package chatdock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const maxJSONRequestBytes = 2 << 20

var errJSONRequestTooLarge = errors.New("JSON request body is too large")

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxJSONRequestBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxJSONRequestBytes {
		return fmt.Errorf("%w: exceeds %d bytes", errJSONRequestTooLarge, maxJSONRequestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON value")
		}
		return fmt.Errorf("decode trailing request body: %w", err)
	}
	return nil
}

func writeJSONResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		logError("write_json_response_failed", err, logFields{"status": status})
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	if status == http.StatusBadRequest && errors.Is(err, errJSONRequestTooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	writeJSONResponse(w, status, map[string]any{"error": err.Error()})
}

func parseOptionalInt(value string, defaultValue int, minValue int, maxValue int, name string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minValue || parsed > maxValue {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minValue, maxValue)
	}
	return parsed, nil
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, value any) error {
	return writeSSEWithID(w, flusher, 0, event, value)
}

func writeSSEWithID(w http.ResponseWriter, flusher http.Flusher, id int, event string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if id > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEHeartbeat(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
