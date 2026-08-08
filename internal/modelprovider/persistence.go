package modelprovider

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

const MetaKey = "model_providers_v1"

type sqlReader interface {
	QueryRow(query string, args ...any) *sql.Row
}

type sqlWriter interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func LoadRecords(reader sqlReader) ([]Record, error) {
	var raw string
	err := reader.QueryRow(`SELECT value FROM meta WHERE key = ?`, MetaKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Record
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		record = NormalizeRecord(record)
		if record.ID == "" || seen[record.ID] {
			continue
		}
		seen[record.ID] = true
		out = append(out, record)
	}
	return out, nil
}

func SaveRecords(writer sqlWriter, records []Record) error {
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	_, err = writer.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, MetaKey, string(raw))
	return err
}
