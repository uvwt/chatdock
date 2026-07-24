package toolschema

import (
	"fmt"
	"math"
)

func ValidateArguments(schema map[string]any, args map[string]any) error {
	if schema == nil {
		return nil
	}
	return validateSchemaValue(schema, args, "arguments")
}

func validateSchemaValue(schema map[string]any, value any, path string) error {
	if err := validateSchemaEnum(schema, value, path); err != nil {
		return err
	}
	types := schemaTypes(schema["type"])
	if len(types) == 0 {
		return validateSchemaChildren(schema, value, path)
	}
	for _, typ := range types {
		if schemaTypeMatches(typ, value) {
			return validateSchemaChildren(schema, value, path)
		}
	}
	return fmt.Errorf("%s must be %s", path, schemaTypeLabel(types))
}

func validateSchemaChildren(schema map[string]any, value any, path string) error {
	switch objectType, _ := schema["type"].(string); objectType {
	case "object":
		return validateSchemaObject(schema, value, path)
	case "array":
		return validateSchemaArray(schema, value, path)
	default:
		if _, ok := value.(map[string]any); ok {
			return validateSchemaObject(schema, value, path)
		}
		if _, ok := value.([]any); ok {
			return validateSchemaArray(schema, value, path)
		}
		return nil
	}
}

func validateSchemaObject(schema map[string]any, value any, path string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be object", path)
	}
	for _, name := range schemaStringList(schema["required"]) {
		if _, ok := object[name]; !ok {
			return fmt.Errorf("%s.%s is required", path, name)
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for name, raw := range object {
		propSchema, ok := props[name].(map[string]any)
		if !ok {
			switch additional := schema["additionalProperties"].(type) {
			case bool:
				if !additional {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
			case map[string]any:
				if err := validateSchemaValue(additional, raw, path+"."+name); err != nil {
					return err
				}
			}
			continue
		}
		if err := validateSchemaValue(propSchema, raw, path+"."+name); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaArray(schema map[string]any, value any, path string) error {
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be array", path)
	}
	itemSchema, _ := schema["items"].(map[string]any)
	if itemSchema == nil {
		return nil
	}
	for i, item := range items {
		if err := validateSchemaValue(itemSchema, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaEnum(schema map[string]any, value any, path string) error {
	rawEnum, ok := schema["enum"]
	if !ok {
		return nil
	}
	for _, allowed := range schemaAnyList(rawEnum) {
		if schemaValuesEqual(allowed, value) {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %v", path, schemaAnyList(rawEnum))
}

func schemaTypeMatches(typ string, value any) bool {
	switch typ {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		return isSchemaInteger(value)
	case "number":
		return isSchemaNumber(value)
	case "null":
		return value == nil
	default:
		return true
	}
}

func isSchemaInteger(value any) bool {
	switch v := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return math.Trunc(v) == v
	case float32:
		return math.Trunc(float64(v)) == float64(v)
	default:
		return false
	}
}

func isSchemaNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func schemaTypes(value any) []string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := item.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func schemaStringList(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func schemaAnyList(value any) []any {
	switch v := value.(type) {
	case []any:
		return v
	case []string:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func schemaTypeLabel(types []string) string {
	if len(types) == 1 {
		return types[0]
	}
	return fmt.Sprintf("one of %v", types)
}

func schemaValuesEqual(left any, right any) bool {
	switch l := left.(type) {
	case string:
		r, ok := right.(string)
		return ok && l == r
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	default:
		if isSchemaNumber(left) && isSchemaNumber(right) {
			return fmt.Sprint(left) == fmt.Sprint(right)
		}
		return fmt.Sprint(left) == fmt.Sprint(right)
	}
}
