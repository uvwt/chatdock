package modelprovider

import (
	"strings"
	"testing"
	"time"

	"chatdock/internal/model"
)

func TestNormalizeRecordMigratesLegacyAPIKey(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	record := NormalizeRecord(Record{
		ID:           " Example Provider ",
		BaseURL:      " https://example.test/v1 ",
		LegacyAPIKey: "legacy-secret",
		DefaultModel: "model-a",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if record.ID != "example-provider" || record.LegacyAPIKey != "" {
		t.Fatalf("normalized record = %#v", record)
	}
	if len(record.APIKeys) != 1 || record.APIKeys[0].APIKey != "legacy-secret" || record.SelectedKeyID != "main" {
		t.Fatalf("legacy key migration = %#v", record.APIKeys)
	}
}

func TestUpdateRecordPreservesMaskedAPIKey(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	enabled := true
	records, created, err := CreateRecord(nil, Input{
		ID:           "primary",
		Name:         "Primary",
		BaseURL:      "https://example.test/v1",
		DefaultModel: "model-a",
		Enabled:      &enabled,
		APIKeys:      []APIKeyInput{{ID: "main", Name: "主 key", APIKey: "real-secret"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Hour)
	records, updated, err := UpdateRecord(records, created.ID, Input{
		Name:          "Primary Updated",
		BaseURL:       "https://example.test/v1",
		DefaultModel:  "model-b",
		Enabled:       &enabled,
		KeyStrategy:   KeyStrategyManual,
		SelectedKeyID: "main",
		APIKeys:       []APIKeyInput{{ID: "main", Name: "主 key", APIKey: "********"}},
	}, later)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || updated.APIKeys[0].APIKey != "real-secret" {
		t.Fatalf("masked update replaced secret: %#v", updated.APIKeys)
	}
	if key, err := SelectedAPIKey(updated); err != nil || key != "real-secret" {
		t.Fatalf("selected key=%q err=%v", key, err)
	}
}

func TestManualSelectionRejectsDisabledKey(t *testing.T) {
	record := NormalizeRecord(Record{
		ID:            "primary",
		BaseURL:       "https://example.test/v1",
		DefaultModel:  "model-a",
		KeyStrategy:   KeyStrategyManual,
		SelectedKeyID: "disabled",
		APIKeys: []APIKeyRecord{{
			ID: "disabled", Name: "Disabled", APIKey: "secret", Enabled: false, Priority: 1,
		}},
	})
	if _, err := SelectedAPIKey(record); err == nil || !strings.Contains(err.Error(), "disabled or empty") {
		t.Fatalf("selection error = %v", err)
	}
}

func TestPublicProjectionMasksSecrets(t *testing.T) {
	record := NormalizeRecord(Record{
		ID:           "primary",
		BaseURL:      "https://example.test/v1",
		DefaultModel: "model-a",
		APIKeys: []APIKeyRecord{{
			ID: "main", Name: "Main", APIKey: "sk-super-secret-value", Enabled: true, Priority: 1,
		}},
	})
	public := Public(record)
	if !public.HasAPIKey || public.APIKeyMasked == "" || strings.Contains(public.APIKeyMasked, "super-secret") {
		t.Fatalf("public provider leaked secret: %#v", public)
	}
	if len(public.APIKeys) != 1 || !public.APIKeys[0].HasAPIKey || public.APIKeys[0].APIKeyMasked == "" {
		t.Fatalf("public keys = %#v", public.APIKeys)
	}
}

func TestModelLimitsAreNormalizedAndExposedPerModel(t *testing.T) {
	enabled := true
	_, record, err := CreateRecord(nil, Input{
		ID:           "limits",
		Name:         "Limits",
		BaseURL:      "https://example.test/v1",
		DefaultModel: "model-a",
		Models:       []string{"model-a", "model-b"},
		ModelLimits: map[string]model.ModelLimit{
			" model-a ": {ContextWindowTokens: 8192, OutputReserveTokens: 1024},
			"model-b":   {ContextWindowTokens: 4096, OutputReserveTokens: 4096},
			"invalid":   {ContextWindowTokens: 0, OutputReserveTokens: 512},
		},
		Enabled: &enabled,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(record.ModelLimits) != 1 || record.ModelLimits["model-a"].ContextWindowTokens != 8192 {
		t.Fatalf("normalized model limits = %#v", record.ModelLimits)
	}
	if limit, ok := LimitForModel(record, "model-a"); !ok || limit.OutputReserveTokens != 1024 {
		t.Fatalf("model-a limit = %#v, ok=%v", limit, ok)
	}
	if _, ok := LimitForModel(record, "model-b"); ok {
		t.Fatal("invalid model-b limit should not be exposed")
	}
	public := Public(record)
	public.ModelLimits["model-a"] = model.ModelLimit{ContextWindowTokens: 1, OutputReserveTokens: 1}
	if record.ModelLimits["model-a"].ContextWindowTokens != 8192 {
		t.Fatal("public model limits leaked mutable map")
	}
}
