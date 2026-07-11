package chatdock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxJSONRequestBytes = 2 << 20

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxJSONRequestBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxJSONRequestBytes {
		return fmt.Errorf("request body exceeds %d bytes", maxJSONRequestBytes)
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
	writeJSONResponse(w, status, map[string]any{"error": err.Error()})
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
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
