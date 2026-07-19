package chatdock

import (
	"encoding/json"
	"fmt"
	"io"
)

func decodeBoundedJSON(reader io.Reader, maxBytes int64, out any) error {
	if maxBytes <= 0 {
		return fmt.Errorf("JSON response limit must be positive")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return fmt.Errorf("read JSON response: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return fmt.Errorf("JSON response exceeds %d bytes", maxBytes)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}
	return nil
}
