package store

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func inputKeysToRecords(inputs []ModelProviderAPIKeyInput, previous []modelProviderAPIKeyRecord, legacyAPIKey string, now time.Time) []modelProviderAPIKeyRecord {
	previousByID := map[string]modelProviderAPIKeyRecord{}
	for _, item := range previous {
		previousByID[item.ID] = item
	}
	out := make([]modelProviderAPIKeyRecord, 0, len(inputs))
	used := map[string]bool{}
	for idx, input := range inputs {
		id := normalizeProviderKeyID(input.ID)
		if id == "" {
			id = normalizeProviderKeyID(input.Name)
		}
		if id == "" {
			id = fmt.Sprintf("key-%d", idx+1)
		}
		base, exists := previousByID[id]
		if !exists {
			base = modelProviderAPIKeyRecord{ID: id, Enabled: true, Priority: idx + 1, CreatedAt: now, UpdatedAt: now}
		}
		if used[id] {
			id = uniqueProviderKeyID(id, used)
			base.ID = id
		}
		used[id] = true
		if strings.TrimSpace(input.Name) != "" {
			base.Name = strings.TrimSpace(input.Name)
		}
		if !isMaskedSecret(input.APIKey) {
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
	if len(out) == 0 && !isMaskedSecret(legacyAPIKey) {
		out = append(out, modelProviderAPIKeyRecord{ID: "main", Name: "主 key", APIKey: strings.TrimSpace(legacyAPIKey), Enabled: true, Priority: 1, CreatedAt: now, UpdatedAt: now})
	}
	return out
}

func upsertLegacyAPIKeyRecord(keys []modelProviderAPIKeyRecord, apiKey string, now time.Time) []modelProviderAPIKeyRecord {
	if strings.TrimSpace(apiKey) == "" {
		return keys
	}
	if len(keys) == 0 {
		return []modelProviderAPIKeyRecord{{ID: "main", Name: "主 key", APIKey: strings.TrimSpace(apiKey), Enabled: true, Priority: 1, CreatedAt: now, UpdatedAt: now}}
	}
	selected := keys[0].ID
	for _, key := range keys {
		if key.Enabled {
			selected = key.ID
			break
		}
	}
	for i := range keys {
		if keys[i].ID == selected {
			keys[i].APIKey = strings.TrimSpace(apiKey)
			keys[i].UpdatedAt = now
			return keys
		}
	}
	return keys
}

func normalizeProviderAPIKeyRecords(keys []modelProviderAPIKeyRecord, now time.Time) []modelProviderAPIKeyRecord {
	out := make([]modelProviderAPIKeyRecord, 0, len(keys))
	used := map[string]bool{}
	for idx, key := range keys {
		key.ID = normalizeProviderKeyID(key.ID)
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

func selectedAPIKeyForRecord(record modelProviderRecord) (string, error) {
	key, ok, err := selectedAPIKeyRecordForRecord(record)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(key.APIKey), nil
}

func selectedAPIKeyRecordForRecord(record modelProviderRecord) (modelProviderAPIKeyRecord, bool, error) {
	keys := sortedProviderKeys(record.APIKeys)
	if len(keys) == 0 {
		return modelProviderAPIKeyRecord{}, false, nil
	}
	if record.KeyStrategy == modelProviderKeyStrategyManual {
		if record.SelectedKeyID == "" {
			return modelProviderAPIKeyRecord{}, false, fmt.Errorf("model provider key_strategy=manual requires selected_key_id: %s", record.ID)
		}
		key, ok := keyByID(keys, record.SelectedKeyID)
		if !ok {
			return modelProviderAPIKeyRecord{}, false, fmt.Errorf("selected api key not found: %s", record.SelectedKeyID)
		}
		if !key.Enabled || strings.TrimSpace(key.APIKey) == "" {
			return modelProviderAPIKeyRecord{}, false, fmt.Errorf("selected api key is disabled or empty: %s", record.SelectedKeyID)
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
	return modelProviderAPIKeyRecord{}, false, fmt.Errorf("model provider has no enabled api keys: %s", record.ID)
}

func publicSelectedKey(record modelProviderRecord) (modelProviderAPIKeyRecord, bool) {
	if record.SelectedKeyID != "" {
		if key, ok := keyByID(record.APIKeys, record.SelectedKeyID); ok {
			return key, true
		}
	}
	return firstEnabledProviderKey(record.APIKeys)
}

func providerHasAnyAPIKey(record modelProviderRecord) bool {
	if strings.TrimSpace(record.APIKey) != "" {
		return true
	}
	for _, key := range record.APIKeys {
		if strings.TrimSpace(key.APIKey) != "" {
			return true
		}
	}
	return false
}

func firstEnabledProviderKey(keys []modelProviderAPIKeyRecord) (modelProviderAPIKeyRecord, bool) {
	for _, key := range sortedProviderKeys(keys) {
		if key.Enabled && strings.TrimSpace(key.APIKey) != "" {
			return key, true
		}
	}
	return modelProviderAPIKeyRecord{}, false
}

func keyByID(keys []modelProviderAPIKeyRecord, id string) (modelProviderAPIKeyRecord, bool) {
	id = normalizeProviderKeyID(id)
	if id == "" {
		return modelProviderAPIKeyRecord{}, false
	}
	for _, key := range keys {
		if key.ID == id {
			return key, true
		}
	}
	return modelProviderAPIKeyRecord{}, false
}

func sortedProviderKeys(keys []modelProviderAPIKeyRecord) []modelProviderAPIKeyRecord {
	out := append([]modelProviderAPIKeyRecord(nil), keys...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}
