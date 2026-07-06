package store

import (
	"encoding/json"
	"strings"
)

func (s *Store) loadModelProviderRecordsLocked() ([]modelProviderRecord, error) {
	raw, err := s.metaValue(modelProvidersMetaKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var records []modelProviderRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, err
	}
	out := make([]modelProviderRecord, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		record = normalizeModelProviderRecord(record)
		if record.ID == "" || seen[record.ID] {
			continue
		}
		seen[record.ID] = true
		out = append(out, record)
	}
	return out, nil
}

func (s *Store) saveModelProviderRecordsLocked(records []modelProviderRecord) error {
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return s.setMetaValue(modelProvidersMetaKey, string(raw))
}
