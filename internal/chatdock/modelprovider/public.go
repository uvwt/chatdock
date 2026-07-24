package modelprovider

import (
	"strings"

	"chatdock/internal/chatdock/model"
)

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 8 {
		if len(runes) <= 2 {
			return strings.Repeat("*", len(runes))
		}
		return string(runes[:1]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1:])
	}
	prefixLen, suffixLen := 6, 4
	if len(runes) > 24 {
		prefixLen, suffixLen = 8, 6
	}
	if len(runes) <= prefixLen+suffixLen {
		return string(runes[:2]) + strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-2:])
	}
	return string(runes[:prefixLen]) + strings.Repeat("*", 8) + string(runes[len(runes)-suffixLen:])
}

func Public(record Record) Provider {
	apiKeys := make([]APIKey, 0, len(record.APIKeys))
	for _, key := range SortedKeys(record.APIKeys) {
		apiKeys = append(apiKeys, APIKey{
			ID:           key.ID,
			Name:         key.Name,
			HasAPIKey:    strings.TrimSpace(key.APIKey) != "",
			APIKeyMasked: maskSecret(key.APIKey),
			Enabled:      key.Enabled,
			Priority:     key.Priority,
			LastStatus:   key.LastStatus,
			LastError:    key.LastError,
			LastTestedAt: key.LastTestedAt,
			CreatedAt:    key.CreatedAt,
			UpdatedAt:    key.UpdatedAt,
		})
	}
	selectedMasked := ""
	if key, ok := publicSelectedKey(record); ok {
		selectedMasked = maskSecret(key.APIKey)
	}
	return Provider{
		ID:            record.ID,
		Name:          record.Name,
		Type:          record.Type,
		BaseURL:       record.BaseURL,
		HasAPIKey:     providerHasAnyAPIKey(record),
		APIKeyMasked:  selectedMasked,
		DefaultModel:  record.DefaultModel,
		Models:        append([]string(nil), record.Models...),
		TimeoutMS:     record.TimeoutMS,
		Enabled:       record.Enabled,
		KeyStrategy:   record.KeyStrategy,
		SelectedKeyID: record.SelectedKeyID,
		APIKeys:       apiKeys,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}

func DisplayName(cfg model.ModelConfig) string {
	host := hostFromURL(cfg.BaseURL)
	if host == "" {
		host = "OpenAI Compatible"
	}
	return host
}
