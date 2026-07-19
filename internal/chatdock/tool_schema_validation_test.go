package chatdock

import (
	"strings"
	"testing"
)

func TestValidateToolArgumentsChecksRequiredNestedAndAdditionalSchemas(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"name", "items"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "enum": []any{"alpha", "beta"}},
			"items": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": []any{"integer", "null"}},
			},
		},
		"additionalProperties": map[string]any{"type": "string"},
	}
	valid := map[string]any{"name": "alpha", "items": []any{float64(1), nil, 3}, "note": "ok"}
	if err := validateToolArguments(schema, valid); err != nil {
		t.Fatalf("valid arguments failed: %v", err)
	}

	cases := map[string]struct {
		args map[string]any
		want string
	}{
		"missing required":    {args: map[string]any{"items": []any{}}, want: "arguments.name is required"},
		"enum":                {args: map[string]any{"name": "gamma", "items": []any{}}, want: "arguments.name must be one of"},
		"nested array item":   {args: map[string]any{"name": "alpha", "items": []any{1.5}}, want: "arguments.items[0] must be one of"},
		"additional property": {args: map[string]any{"name": "alpha", "items": []any{}, "note": 42}, want: "arguments.note must be string"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateToolArguments(schema, tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validation error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateToolArgumentsRejectsDisallowedUnknownFields(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"known": map[string]any{"type": "boolean"}},
		"additionalProperties": false,
	}
	if err := validateToolArguments(schema, map[string]any{"known": true}); err != nil {
		t.Fatal(err)
	}
	if err := validateToolArguments(schema, map[string]any{"unknown": true}); err == nil || !strings.Contains(err.Error(), "arguments.unknown is not allowed") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestSchemaNumberAndIntegerRecognition(t *testing.T) {
	for _, value := range []any{1, int64(2), float64(3)} {
		if !isSchemaInteger(value) || !isSchemaNumber(value) {
			t.Fatalf("integer value %#v was not recognized", value)
		}
	}
	if isSchemaInteger(1.5) || !isSchemaNumber(1.5) || isSchemaNumber("1") {
		t.Fatal("number classification returned an unexpected result")
	}
}
