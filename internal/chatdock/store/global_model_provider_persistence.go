package store

import (
	"encoding/json"
	"strings"
)

func (s *Store) loadModelProviderRecordsLocked() ([]modelProviderRecord, error) {
	return loadModelProviderRecordsWith(s.db)
}

func loadModelProviderRecordsWith(reader sqlQueryer) ([]modelProviderRecord, error) {
	raw, err := metaValueWith(reader, modelProvidersMetaKey)
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
	return saveModelProviderRecordsWith(s.db, records)
}

func saveModelProviderRecordsWith(writer sqlWriter, records []modelProviderRecord) error {
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return setMetaValueWith(writer, modelProvidersMetaKey, string(raw))
}

func (s *Store) migrateModelProviderAPIKeys() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := s.metaValue(modelProvidersMetaKey)
	if err != nil {
		return err
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var records []modelProviderRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return err
	}
	migrated := false
	for index := range records {
		if strings.TrimSpace(records[index].LegacyAPIKey) != "" {
			migrated = true
		}
		records[index] = normalizeModelProviderRecord(records[index])
	}
	if !migrated {
		return nil
	}
	return s.saveModelProviderRecordsLocked(records)
}
