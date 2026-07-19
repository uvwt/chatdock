package store

import (
	"strings"
	"testing"
)

func TestNormalizeWorkspaceIDAllowsUnicodeWithinLimit(t *testing.T) {
	name := strings.Repeat("工作区", 42) + "终"
	if len([]rune(name)) != 127 {
		t.Fatalf("test workspace length = %d", len([]rune(name)))
	}
	got, err := normalizeWorkspaceID("  " + name + "  ")
	if err != nil || got != name {
		t.Fatalf("workspace ID = %q, error=%v", got, err)
	}
}

func TestNormalizeWorkspaceIDRejectsOversizedPathsAndControls(t *testing.T) {
	cases := map[string]string{
		strings.Repeat("界", maxWorkspaceIDRunes+1): "exceeds",
		"../secret":   "path separators",
		"line\nbreak": "control characters",
	}
	for value, want := range cases {
		if _, err := normalizeWorkspaceID(value); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("workspace ID %q error = %v, want %q", value, err, want)
		}
	}
}
