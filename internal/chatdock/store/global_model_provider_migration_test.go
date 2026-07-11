package store

import (
	"encoding/json"
	"testing"
)

func TestStoreMigratesLegacyModelProviderAPIKey(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `[{"id":"legacy","name":"Legacy","type":"openai-compatible","base_url":"https://example.test/v1","api_key":"legacy-secret","default_model":"demo","models":["demo"],"timeout_ms":120000,"enabled":true}]`
	if err := store.setMetaValue(modelProvidersMetaKey, legacy); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	raw, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("unexpected migrated records: %#v", records)
	}
	if _, exists := records[0]["api_key"]; exists {
		t.Fatalf("legacy top-level api_key should be removed: %s", raw)
	}
	keys, _ := records[0]["api_keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("legacy key should migrate into api_keys: %s", raw)
	}
	key, _ := keys[0].(map[string]any)
	if key["id"] != "main" || key["api_key"] != "legacy-secret" {
		t.Fatalf("unexpected migrated key: %#v", key)
	}

	cfg, ok, err := store.ModelProviderConfig("legacy")
	if err != nil || !ok {
		t.Fatalf("migrated provider config unavailable: ok=%v err=%v", ok, err)
	}
	if cfg.APIKey != "legacy-secret" || cfg.Model != "demo" {
		t.Fatalf("unexpected migrated provider config: %#v", cfg)
	}
}

func TestCreateModelProviderPersistsOnlyAPIKeys(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	enabled := true
	provider, err := store.CreateModelProvider(ModelProviderInput{
		ID:           "current",
		Name:         "Current",
		BaseURL:      "https://example.test/v1",
		DefaultModel: "demo",
		Enabled:      true,
		APIKeys: []ModelProviderAPIKeyInput{
			{ID: "main", Name: "主 key", APIKey: "current-secret", Enabled: &enabled, Priority: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record["id"] != provider.ID {
			continue
		}
		if _, exists := record["api_key"]; exists {
			t.Fatalf("current provider must not persist top-level api_key: %#v", record)
		}
		keys, _ := record["api_keys"].([]any)
		if len(keys) != 1 {
			t.Fatalf("current provider key missing: %#v", record)
		}
		return
	}
	t.Fatalf("provider %q not found in %s", provider.ID, raw)
}
