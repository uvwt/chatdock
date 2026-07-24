package modelprovider

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func inputKeysToRecords(inputs []APIKeyInput, previous []APIKeyRecord, now time.Time) []APIKeyRecord {
	previousByID := map[string]APIKeyRecord{}
	for _, item := range previous {
		previousByID[item.ID] = item
	}
	out := make([]APIKeyRecord, 0, len(inputs))
	used := map[string]bool{}
	for idx, input := range inputs {
		id := NormalizeKeyID(input.ID)
		if id == "" {
			id = NormalizeKeyID(input.Name)
		}
		if id == "" {
			id = fmt.Sprintf("key-%d", idx+1)
		}
		base, exists := previousByID[id]
		if !exists {
			base = APIKeyRecord{ID: id, Enabled: true, Priority: idx + 1, CreatedAt: now, UpdatedAt: now}
		}
		if used[id] {
			id = uniqueProviderKeyID(id, used)
			base.ID = id
		}
		used[id] = true
		if strings.TrimSpace(input.Name) != "" {
			base.Name = strings.TrimSpace(input.Name)
		}
		if !IsMaskedSecret(input.APIKey) {
			base.APIKey = strings.TrimSpace(input.APIKey)
		}
		if input.Enabled != nil {
			base.Enabled = *input.Enabled
		}
		if input.Priority > 0 {
			base.Priority = input.Priority
		} else if base.Priority <= 0 {
			base.Priority = idx + 1
		}
		base.UpdatedAt = now
		out = append(out, base)
	}
	return out
}

func UpsertAPIKey(keys []APIKeyRecord, selectedKeyID string, apiKey string, now time.Time) []APIKeyRecord {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || IsMaskedSecret(apiKey) {
		return keys
	}
	selectedKeyID = NormalizeKeyID(selectedKeyID)
	if len(keys) == 0 {
		if selectedKeyID == "" {
			selectedKeyID = "main"
		}
		return []APIKeyRecord{{ID: selectedKeyID, Name: "主 key", APIKey: apiKey, Enabled: true, Priority: 1, CreatedAt: now, UpdatedAt: now}}
	}
	if selectedKeyID == "" {
		if selected, ok := firstEnabledProviderKey(keys); ok {
			selectedKeyID = selected.ID
		} else {
			selectedKeyID = keys[0].ID
		}
	}
	for index := range keys {
		if keys[index].ID != selectedKeyID {
			continue
		}
		keys[index].APIKey = apiKey
		keys[index].UpdatedAt = now
		return keys
	}
	return append(keys, APIKeyRecord{ID: selectedKeyID, Name: selectedKeyID, APIKey: apiKey, Enabled: true, Priority: len(keys) + 1, CreatedAt: now, UpdatedAt: now})
}

func normalizeAPIKeyRecords(keys []APIKeyRecord, now time.Time) []APIKeyRecord {
	out := make([]APIKeyRecord, 0, len(keys))
	used := map[string]bool{}
	for idx, key := range keys {
		key.ID = NormalizeKeyID(key.ID)
		if key.ID == "" {
			key.ID = fmt.Sprintf("key-%d", idx+1)
		}
		if used[key.ID] {
			key.ID = uniqueProviderKeyID(key.ID, used)
		}
		used[key.ID] = true
		key.Name = strings.TrimSpace(key.Name)
		key.APIKey = strings.TrimSpace(key.APIKey)
		if key.Name == "" {
			key.Name = key.ID
		}
		if key.Priority <= 0 {
			key.Priority = idx + 1
		}
		if key.CreatedAt.IsZero() {
			key.CreatedAt = now
		}
		if key.UpdatedAt.IsZero() {
			key.UpdatedAt = key.CreatedAt
		}
		out = append(out, key)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func SelectedAPIKey(record Record) (string, error) {
	key, ok, err := SelectedAPIKeyRecord(record)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(key.APIKey), nil
}

func SelectedAPIKeyRecord(record Record) (APIKeyRecord, bool, error) {
	keys := SortedKeys(record.APIKeys)
	if len(keys) == 0 {
		return APIKeyRecord{}, false, nil
	}
	if record.KeyStrategy == KeyStrategyManual {
		if record.SelectedKeyID == "" {
			return APIKeyRecord{}, false, fmt.Errorf("model provider key_strategy=manual requires selected_key_id: %s", record.ID)
		}
		key, ok := keyByID(keys, record.SelectedKeyID)
		if !ok {
			return APIKeyRecord{}, false, fmt.Errorf("selected api key not found: %s", record.SelectedKeyID)
		}
		if !key.Enabled || strings.TrimSpace(key.APIKey) == "" {
			return APIKeyRecord{}, false, fmt.Errorf("selected api key is disabled or empty: %s", record.SelectedKeyID)
		}
		return key, true, nil
	}
	if record.SelectedKeyID != "" {
		if key, ok := keyByID(keys, record.SelectedKeyID); ok && key.Enabled && strings.TrimSpace(key.APIKey) != "" {
			return key, true, nil
		}
	}
	for _, key := range keys {
		if key.Enabled && strings.TrimSpace(key.APIKey) != "" {
			return key, true, nil
		}
	}
	return APIKeyRecord{}, false, fmt.Errorf("model provider has no enabled api keys: %s", record.ID)
}

func publicSelectedKey(record Record) (APIKeyRecord, bool) {
	if record.SelectedKeyID != "" {
		if key, ok := keyByID(record.APIKeys, record.SelectedKeyID); ok {
			return key, true
		}
	}
	return firstEnabledProviderKey(record.APIKeys)
}

func providerHasAnyAPIKey(record Record) bool {
	for _, key := range record.APIKeys {
		if strings.TrimSpace(key.APIKey) != "" {
			return true
		}
	}
	return false
}

func firstEnabledProviderKey(keys []APIKeyRecord) (APIKeyRecord, bool) {
	for _, key := range SortedKeys(keys) {
		if key.Enabled && strings.TrimSpace(key.APIKey) != "" {
			return key, true
		}
	}
	return APIKeyRecord{}, false
}

func keyByID(keys []APIKeyRecord, id string) (APIKeyRecord, bool) {
	id = NormalizeKeyID(id)
	if id == "" {
		return APIKeyRecord{}, false
	}
	for _, key := range keys {
		if key.ID == id {
			return key, true
		}
	}
	return APIKeyRecord{}, false
}

func SortedKeys(keys []APIKeyRecord) []APIKeyRecord {
	out := append([]APIKeyRecord(nil), keys...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}
