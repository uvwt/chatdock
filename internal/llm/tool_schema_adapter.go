package llm

import (
	"sort"
	"strings"
)

// adaptToolSchemaForModelAPI 保留原始 JSON Schema，同时生成模型接口普遍能接受的兼容子集。
// 工具执行前仍使用原始 schema 校验，兼容转换只影响发送给模型的工具定义。
func adaptToolSchemaForModelAPI(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}

	adapted := adaptToolSchemaObject(schema, nil)
	flattenTopLevelToolSchemaCompositions(adapted)
	if _, ok := adapted["type"]; !ok {
		adapted["type"] = "object"
	}
	if _, ok := adapted["properties"]; !ok {
		adapted["properties"] = map[string]any{}
	}
	return adapted
}

func adaptToolSchemaObject(schema map[string]any, inheritedProperties map[string]any) map[string]any {
	adapted := make(map[string]any, len(schema)+1)
	for key, value := range schema {
		adapted[key] = cloneToolSchemaValue(value)
	}

	properties, _ := adapted["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
	}
	for _, name := range toolSchemaStringList(adapted["required"]) {
		if _, exists := properties[name]; exists {
			continue
		}
		if inherited, exists := inheritedProperties[name]; exists {
			properties[name] = cloneToolSchemaValue(inherited)
		}
	}
	if _, declared := schema["properties"]; declared || len(properties) > 0 {
		adapted["properties"] = properties
	}

	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		branches, ok := adapted[keyword].([]any)
		if !ok {
			continue
		}
		for index, value := range branches {
			branch, ok := value.(map[string]any)
			if !ok {
				continue
			}
			branches[index] = adaptToolSchemaObject(branch, properties)
		}
		adapted[keyword] = branches
	}
	return adapted
}

// flattenTopLevelToolSchemaCompositions 将部分模型供应商不支持的顶层组合关键字
// 降级为普通 object schema，并把无法直接表达的选择关系保留在描述中。
func flattenTopLevelToolSchemaCompositions(schema map[string]any) {
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
	}
	required := toolSchemaStringList(schema["required"])
	hints := make([]string, 0, 3)

	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		branches, ok := schema[keyword].([]any)
		if !ok {
			continue
		}

		groups := make([]string, 0, len(branches))
		var commonRequired map[string]bool
		for index, value := range branches {
			branch, ok := value.(map[string]any)
			if !ok {
				commonRequired = intersectToolSchemaFields(commonRequired, nil, index == 0)
				continue
			}
			if branchProperties, ok := branch["properties"].(map[string]any); ok {
				for name, definition := range branchProperties {
					if _, exists := properties[name]; !exists {
						properties[name] = cloneToolSchemaValue(definition)
					}
				}
			}

			branchRequired := toolSchemaStringList(branch["required"])
			if len(branchRequired) > 0 {
				groups = append(groups, strings.Join(branchRequired, ", "))
			}
			if keyword == "allOf" {
				required = appendUniqueToolSchemaFields(required, branchRequired...)
				continue
			}
			commonRequired = intersectToolSchemaFields(commonRequired, branchRequired, index == 0)
		}

		if keyword != "allOf" {
			commonNames := make([]string, 0, len(commonRequired))
			for name := range commonRequired {
				commonNames = append(commonNames, name)
			}
			sort.Strings(commonNames)
			for _, name := range commonNames {
				required = appendUniqueToolSchemaFields(required, name)
			}
		}
		if hint := topLevelToolSchemaCompositionHint(keyword, groups); hint != "" {
			hints = append(hints, hint)
		}
		delete(schema, keyword)
	}

	schema["properties"] = properties
	if len(required) > 0 {
		schema["required"] = required
	}
	if len(hints) > 0 {
		description, _ := schema["description"].(string)
		schema["description"] = strings.TrimSpace(strings.Join([]string{description, strings.Join(hints, " ")}, " "))
	}
}

func appendUniqueToolSchemaFields(fields []string, names ...string) []string {
	seen := make(map[string]bool, len(fields)+len(names))
	for _, field := range fields {
		seen[field] = true
	}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		fields = append(fields, name)
		seen[name] = true
	}
	return fields
}

func intersectToolSchemaFields(current map[string]bool, fields []string, first bool) map[string]bool {
	next := make(map[string]bool, len(fields))
	for _, field := range fields {
		next[field] = true
	}
	if first {
		return next
	}
	for field := range current {
		if !next[field] {
			delete(current, field)
		}
	}
	return current
}

func topLevelToolSchemaCompositionHint(keyword string, groups []string) string {
	if len(groups) == 0 {
		return ""
	}
	joined := "[" + strings.Join(groups, "]; [") + "]"
	switch keyword {
	case "oneOf":
		return "Input constraint: satisfy exactly one required-field group: " + joined + "."
	case "anyOf":
		return "Input constraint: satisfy at least one required-field group: " + joined + "."
	case "allOf":
		return "Input constraint: satisfy every required-field group: " + joined + "."
	default:
		return ""
	}
}

func cloneToolSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return adaptToolSchemaObject(typed, nil)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneToolSchemaValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func toolSchemaStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}
