package llm

// adaptToolSchemaForModelAPI 保留原始 JSON Schema 语义，同时兼容部分模型网关对组合分支的额外限制。
// 这些网关要求 oneOf/anyOf/allOf 分支中的 required 字段必须同时出现在该分支自己的 properties 中。
func adaptToolSchemaForModelAPI(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}

	adapted := adaptToolSchemaObject(schema, nil)
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
		return typed
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
