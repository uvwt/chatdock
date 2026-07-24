package modelprovider

import (
	"fmt"
	"strings"
	"time"
)

func CreateRecord(records []Record, input Input, now time.Time) ([]Record, Record, error) {
	id := NormalizeID(input.ID)
	if id == "" {
		id = NormalizeID(input.Name)
	}
	if id == "" {
		id = NormalizeID(hostFromURL(input.BaseURL))
	}
	if id == "" {
		id = fmt.Sprintf("provider-%d", now.Unix())
	}
	id = UniqueID(id, records)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	record := Record{
		ID: id, Name: strings.TrimSpace(input.Name), Type: strings.TrimSpace(input.Type), BaseURL: strings.TrimSpace(input.BaseURL),
		DefaultModel: strings.TrimSpace(input.DefaultModel), Models: NormalizeModelNames(input.Models, input.DefaultModel),
		TimeoutMS: input.TimeoutMS, Enabled: enabled, CreatedAt: now, UpdatedAt: now,
	}
	record.KeyStrategy = input.KeyStrategy
	record.SelectedKeyID = input.SelectedKeyID
	record.APIKeys = inputKeysToRecords(input.APIKeys, nil, now)
	record = NormalizeRecord(record)
	if err := validateRecord(record); err != nil {
		return nil, Record{}, err
	}
	return append(records, record), record, nil
}

func UpdateRecord(records []Record, id string, input Input, now time.Time) ([]Record, Record, error) {
	id = NormalizeID(id)
	if id == "" {
		return nil, Record{}, fmt.Errorf("model provider id is required")
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		record := records[i]
		record.Name = strings.TrimSpace(input.Name)
		record.Type = strings.TrimSpace(input.Type)
		record.BaseURL = strings.TrimSpace(input.BaseURL)
		record.DefaultModel = strings.TrimSpace(input.DefaultModel)
		record.Models = NormalizeModelNames(input.Models, input.DefaultModel)
		record.TimeoutMS = input.TimeoutMS
		if input.Enabled != nil {
			record.Enabled = *input.Enabled
		}
		record.KeyStrategy = strings.TrimSpace(input.KeyStrategy)
		record.SelectedKeyID = NormalizeKeyID(input.SelectedKeyID)
		if input.APIKeys != nil {
			record.APIKeys = inputKeysToRecords(input.APIKeys, record.APIKeys, now)
		}
		record.UpdatedAt = now
		record = NormalizeRecord(record)
		if err := validateRecord(record); err != nil {
			return nil, Record{}, err
		}
		records[i] = record
		return records, record, nil
	}
	return nil, Record{}, fmt.Errorf("model provider not found: %s", id)
}

func RecordExists(records []Record, id string) bool {
	if id == "" {
		return false
	}
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}
