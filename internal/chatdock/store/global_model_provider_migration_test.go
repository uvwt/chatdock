package store

import (
	"encoding/json"
	"testing"

	"chatdock/internal/chatdock/model"
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
		Enabled:      &enabled,
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

func TestModelProviderEnabledOptionalSemantics(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateModelProvider(ModelProviderInput{
		ID:           "enabled-default",
		Name:         "Enabled Default",
		BaseURL:      "https://example.test/v1",
		DefaultModel: "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Enabled {
		t.Fatal("omitted enabled must default to true on create")
	}

	disabled := false
	createdDisabled, err := store.CreateModelProvider(ModelProviderInput{
		ID:           "disabled-explicit",
		Name:         "Disabled Explicit",
		BaseURL:      "https://disabled.example.test/v1",
		DefaultModel: "demo",
		Enabled:      &disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if createdDisabled.Enabled {
		t.Fatal("explicit false must be persisted during create")
	}

	updated, err := store.UpdateModelProvider(created.ID, ModelProviderInput{
		Name:          "Renamed",
		Type:          created.Type,
		BaseURL:       created.BaseURL,
		DefaultModel:  created.DefaultModel,
		Models:        created.Models,
		TimeoutMS:     created.TimeoutMS,
		KeyStrategy:   created.KeyStrategy,
		SelectedKeyID: created.SelectedKeyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled {
		t.Fatal("omitted enabled must preserve current value on update")
	}
}

func TestEnsureGlobalModelProvidersRollsBackAllWorkspaces(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "research"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DELETE FROM meta WHERE key = ?`, modelProvidersMetaKey); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for _, workspaceID := range []string{defaultWorkspaceID, "research"} {
		cfg, err := store.ModelConfig(workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ProviderID = ""
		if err := store.setWorkspaceJSONLocked(workspaceID, "config", cfg); err != nil {
			t.Fatal(err)
		}
		raw, ok, err := store.getWorkspaceRawLocked(workspaceID, "config")
		if err != nil || !ok {
			t.Fatalf("read %s config: ok=%v err=%v", workspaceID, ok, err)
		}
		before[workspaceID] = raw
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_research_provider_migration
BEFORE UPDATE ON workspace_kv
WHEN OLD.workspace_id = 'research' AND OLD.key = 'config'
BEGIN
  SELECT RAISE(ABORT, 'forced provider migration failure');
END`); err != nil {
		t.Fatal(err)
	}

	if err := store.EnsureGlobalModelProviders(); err == nil {
		t.Fatal("expected provider migration failure")
	}
	meta, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	if meta != "" {
		t.Fatal("provider metadata persisted despite migration rollback")
	}
	for workspaceID, want := range before {
		got, ok, err := store.getWorkspaceRawLocked(workspaceID, "config")
		if err != nil || !ok {
			t.Fatalf("read %s after rollback: ok=%v err=%v", workspaceID, ok, err)
		}
		if got != want {
			t.Fatalf("workspace %s config changed despite migration rollback", workspaceID)
		}
	}
}

func TestUpsertModelProviderRollsBackWhenWorkspaceDefaultSaveFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	beforeMeta, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	beforeConfig, ok, err := store.getWorkspaceRawLocked(defaultWorkspaceID, "config")
	if err != nil || !ok {
		t.Fatalf("read initial config: ok=%v err=%v", ok, err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_provider_workspace_default
BEFORE UPDATE ON workspace_kv
WHEN OLD.workspace_id = 'default' AND OLD.key = 'config'
BEGIN
  SELECT RAISE(ABORT, 'forced workspace default failure');
END`); err != nil {
		t.Fatal(err)
	}
	enabled := true
	_, _, err = store.UpsertModelProvider(defaultWorkspaceID, "atomic-provider", ModelProviderInput{
		ID:           "atomic-provider",
		Name:         "Atomic Provider",
		Type:         "openai-compatible",
		BaseURL:      "https://atomic.example.test/v1",
		DefaultModel: "atomic-model",
		Models:       []string{"atomic-model"},
		TimeoutMS:    120000,
		Enabled:      &enabled,
		KeyStrategy:  "auto",
	}, true, "atomic-model")
	if err == nil {
		t.Fatal("expected workspace default save failure")
	}
	afterMeta, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	if afterMeta != beforeMeta {
		t.Fatal("provider metadata survived failed workspace default transaction")
	}
	afterConfig, ok, err := store.getWorkspaceRawLocked(defaultWorkspaceID, "config")
	if err != nil || !ok {
		t.Fatalf("read config after rollback: ok=%v err=%v", ok, err)
	}
	if afterConfig != beforeConfig {
		t.Fatal("workspace config changed despite provider transaction rollback")
	}
}
