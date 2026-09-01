package modelprovider

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"chatdock/internal/model"
)

func NormalizeRecord(record Record) Record {
	now := time.Now()
	record.ID = NormalizeID(record.ID)
	record.Name = strings.TrimSpace(record.Name)
	record.Type = strings.TrimSpace(record.Type)
	record.BaseURL = strings.TrimSpace(record.BaseURL)
	legacyAPIKey := strings.TrimSpace(record.LegacyAPIKey)
	record.LegacyAPIKey = ""
	record.DefaultModel = strings.TrimSpace(record.DefaultModel)
	record.Models = NormalizeModelNames(record.Models, record.DefaultModel)
	record.ModelLimits = normalizeModelLimits(record.ModelLimits)
	record.KeyStrategy = normalizeProviderKeyStrategy(record.KeyStrategy)
	record.SelectedKeyID = NormalizeKeyID(record.SelectedKeyID)
	if len(record.APIKeys) == 0 && legacyAPIKey != "" {
		record.APIKeys = []APIKeyRecord{{ID: "main", Name: "主 key", APIKey: legacyAPIKey, Enabled: true, Priority: 1, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}}
	}
	record.APIKeys = normalizeAPIKeyRecords(record.APIKeys, now)
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

func normalizeModelLimits(limits map[string]model.ModelLimit) map[string]model.ModelLimit {
	if len(limits) == 0 {
		return nil
	}
	out := make(map[string]model.ModelLimit, len(limits))
	for name, limit := range limits {
		name = strings.TrimSpace(name)
		if name == "" || limit.ContextWindowTokens <= 0 || limit.OutputReserveTokens <= 0 {
			continue
		}
		if limit.OutputReserveTokens >= limit.ContextWindowTokens {
			continue
		}
		out[name] = limit
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func LimitForModel(record Record, modelName string) (model.ModelLimit, bool) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = record.DefaultModel
	}
	limit, ok := record.ModelLimits[modelName]
	return limit, ok
}

func validateRecord(record Record) error {
	if record.ID == "" {
		return fmt.Errorf("model provider id is required")
	}
	if strings.TrimSpace(record.BaseURL) == "" {
		return fmt.Errorf("model provider base_url is required")
	}
	if strings.TrimSpace(record.DefaultModel) == "" {
		return fmt.Errorf("model provider default_model is required")
	}
	if record.KeyStrategy == KeyStrategyManual {
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
	case KeyStrategyManual:
		return KeyStrategyManual
	default:
		return KeyStrategyAuto
	}
}

func NormalizeID(value string) string {
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

func NormalizeKeyID(value string) string {
	return NormalizeID(value)
}

func UniqueID(id string, records []Record) string {
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

func NormalizeModelNames(models []string, fallback string) []string {
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

func IsMaskedSecret(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	return strings.Contains(value, "*")
}
