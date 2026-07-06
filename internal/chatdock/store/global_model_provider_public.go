package store

import (
	"strings"

	"chatdock/internal/chatdock/model"
)

func publicModelProvider(record modelProviderRecord) ModelProvider {
	apiKeys := make([]ModelProviderAPIKey, 0, len(record.APIKeys))
	for _, key := range sortedProviderKeys(record.APIKeys) {
		apiKeys = append(apiKeys, ModelProviderAPIKey{
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
	} else {
		selectedMasked = maskSecret(record.APIKey)
	}
	return ModelProvider{
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

func providerIDFromWorkspace(workspace string) string {
	id := normalizeProviderID(workspace)
	if id == "" {
		return defaultPromptName
	}
	return id
}

func providerDisplayName(workspace string, cfg model.ModelConfig) string {
	name := strings.TrimSpace(workspace)
	host := hostFromURL(cfg.BaseURL)
	if host == "" {
		host = "OpenAI Compatible"
	}
	if name == "" {
		return host
	}
	return name + " · " + host
}
