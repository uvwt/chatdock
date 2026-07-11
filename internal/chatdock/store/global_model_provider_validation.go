package store

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
)

func normalizeModelProviderRecord(record modelProviderRecord) modelProviderRecord {
	now := time.Now()
	record.ID = normalizeProviderID(record.ID)
	record.Name = strings.TrimSpace(record.Name)
	record.Type = strings.TrimSpace(record.Type)
	record.BaseURL = strings.TrimSpace(record.BaseURL)
	legacyAPIKey := strings.TrimSpace(record.LegacyAPIKey)
	record.LegacyAPIKey = ""
	record.DefaultModel = strings.TrimSpace(record.DefaultModel)
	record.Models = normalizeProviderModelNames(record.Models, record.DefaultModel)
	record.KeyStrategy = normalizeProviderKeyStrategy(record.KeyStrategy)
	record.SelectedKeyID = normalizeProviderKeyID(record.SelectedKeyID)
	if len(record.APIKeys) == 0 && legacyAPIKey != "" {
		record.APIKeys = []modelProviderAPIKeyRecord{{ID: "main", Name: "主 key", APIKey: legacyAPIKey, Enabled: true, Priority: 1, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}}
	}
	record.APIKeys = normalizeProviderAPIKeyRecords(record.APIKeys, now)
	if record.SelectedKeyID == "" {
		if key, ok := firstEnabledProviderKey(record.APIKeys); ok {
			record.SelectedKeyID = key.ID
		}
	}
	if record.Type == "" {
		record.Type = "openai-compatible"
	}
	if record.Name == "" {
		record.Name = record.ID
	}
	if record.TimeoutMS <= 0 {
		record.TimeoutMS = 120000
	}
	if record.DefaultModel == "" && len(record.Models) > 0 {
		record.DefaultModel = record.Models[0]
	}
	if len(record.Models) == 0 && record.DefaultModel != "" {
		record.Models = []string{record.DefaultModel}
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	return record
}

func validateModelProviderRecord(record modelProviderRecord) error {
	if record.ID == "" {
		return fmt.Errorf("model provider id is required")
	}
	if strings.TrimSpace(record.BaseURL) == "" {
		return fmt.Errorf("model provider base_url is required")
	}
	if strings.TrimSpace(record.DefaultModel) == "" {
		return fmt.Errorf("model provider default_model is required")
	}
	if record.KeyStrategy == modelProviderKeyStrategyManual {
		if record.SelectedKeyID == "" && len(record.APIKeys) > 0 {
			return fmt.Errorf("selected_key_id is required when key_strategy is manual")
		}
		if record.SelectedKeyID != "" {
			if _, ok := keyByID(record.APIKeys, record.SelectedKeyID); !ok {
				return fmt.Errorf("selected api key not found: %s", record.SelectedKeyID)
			}
		}
	}
	return nil
}

func normalizeProviderKeyStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case modelProviderKeyStrategyManual:
		return modelProviderKeyStrategyManual
	default:
		return modelProviderKeyStrategyAuto
	}
}

func normalizeProviderID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '_' || r == '-' {
			if !lastDash {
				b.WriteRune(r)
				lastDash = r == '-'
			}
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-_")
}

func normalizeProviderKeyID(value string) string {
	return normalizeProviderID(value)
}

func uniqueProviderID(id string, records []modelProviderRecord) string {
	used := map[string]bool{}
	for _, record := range records {
		used[record.ID] = true
	}
	if !used[id] {
		return id
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", id, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func uniqueProviderKeyID(id string, used map[string]bool) string {
	base := id
	if base == "" {
		base = "key"
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func normalizeProviderModelNames(models []string, fallback string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	add(fallback)
	for _, name := range models {
		add(name)
	}
	return out
}

func hostFromURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://")
	}
	return parsed.Host
}

func isMaskedSecret(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	return strings.Contains(value, "*")
}
