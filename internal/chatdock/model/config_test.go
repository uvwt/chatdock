package model

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLegacyEnableThinkingConfigIsIgnored(t *testing.T) {
	var cfg ModelConfig
	if err := json.Unmarshal([]byte(`{"model":"legacy-model","enable_thinking":true,"hide_thinking":true}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "legacy-model" || !cfg.HideThinking {
		t.Fatalf("legacy config did not preserve supported fields: %#v", cfg)
	}

	publicJSON, err := json.Marshal(ToPublicModelConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(publicJSON, []byte("enable_thinking")) {
		t.Fatalf("legacy thinking switch leaked into public config: %s", publicJSON)
	}
}
