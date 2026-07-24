package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/mcp"
	"chatdock/internal/store"
)

const (
	builtinToolListModelProviders   = "chatdock_model_providers_list"
	builtinToolSaveModelProvider    = "chatdock_model_provider_save"
	builtinToolDeleteModelProvider  = "chatdock_model_provider_delete"
	builtinToolTestModelProvider    = "chatdock_model_provider_test"
	builtinToolServerModelProviders = "chatdock"
)

func builtinModelProviderTools() []mcp.MCPTool {
	keyProps := map[string]any{
		"id":       map[string]any{"type": "string", "description": "Key id，例如 main、backup；为空时按名称自动生成"},
		"name":     map[string]any{"type": "string", "description": "Key 显示名称，例如 主 key、备用 key"},
		"api_key":  map[string]any{"type": "string", "description": "API Key 明文。留空或传 ******** 表示保留原 Key。工具结果不会回显明文。"},
		"enabled":  map[string]any{"type": "boolean", "description": "是否启用该 Key，默认 true"},
		"priority": map[string]any{"type": "integer", "description": "auto 策略下优先级，数字越小越优先"},
	}
	providerProps := map[string]any{
		"id":                    map[string]any{"type": "string", "description": "供应商 id；存在则编辑，不存在则创建；为空时创建并自动生成"},
		"name":                  map[string]any{"type": "string", "description": "供应商显示名称，例如 OpenAI、Volc Ark、Ollama、LM Studio"},
		"base_url":              map[string]any{"type": "string", "description": "OpenAI 兼容 Base URL，例如 https://api.openai.com/v1 或 http://127.0.0.1:11434/v1"},
		"default_model":         map[string]any{"type": "string", "description": "默认模型名称，例如 gpt-4o-mini、doubao-seed-1-6、qwen3-coder"},
		"models":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "可用模型列表，必须由用户手动添加或确认。"},
		"timeout_ms":            map[string]any{"type": "integer", "description": "请求超时毫秒，默认 120000"},
		"enabled":               map[string]any{"type": "boolean", "description": "是否启用供应商，默认 true"},
		"key_strategy":          map[string]any{"type": "string", "enum": []string{"auto", "manual"}, "description": "Key 选择策略。auto=优先 selected_key_id，失败/不可用时按 priority 选择其他启用 Key；manual=只使用 selected_key_id。默认 auto。"},
		"selected_key_id":       map[string]any{"type": "string", "description": "当前选中的 Key id。manual 策略必须有效；auto 策略下会优先使用它。"},
		"api_keys":              map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": keyProps}, "description": "同一供应商的多个 Key。结果只返回 has_api_key/api_key_masked 和测试状态，不返回明文。"},
		"set_as_global_default": map[string]any{"type": "boolean", "description": "保存后是否设为全局默认供应商"},
		"global_model":          map[string]any{"type": "string", "description": "设为全局默认供应商时使用的模型；为空用 default_model"},
	}
	return []mcp.MCPTool{
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_providers_list",
			FullName:    builtinToolListModelProviders,
			Title:       "查询模型供应商",
			Description: "查询全局模型供应商。可传 id 精确查询，或传 query 按名称、Base URL、模型名模糊过滤。结果只返回 has_api_key/api_key_masked 和 key 状态，不返回明文 API Key。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "query": map[string]any{"type": "string"}}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_save",
			FullName:    builtinToolSaveModelProvider,
			Title:       "保存模型供应商",
			Description: "新增或编辑 OpenAI 兼容模型供应商；也可启用/停用、设置全局默认供应商，并在 api_keys 中维护多个 Key。",
			InputSchema: map[string]any{"type": "object", "properties": providerProps},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_test",
			FullName:    builtinToolTestModelProvider,
			Title:       "测试模型供应商",
			Description: "Test real chat connectivity with /chat/completions. fetch_models=true only returns candidate_models from /models; candidate models are not saved automatically. Only chat success marks the provider/key usable.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "供应商 id"}, "model": map[string]any{"type": "string", "description": "可选，临时测试的模型名"}, "selected_key_id": map[string]any{"type": "string", "description": "只测试指定 Key"}, "all_keys": map[string]any{"type": "boolean", "description": "测试所有启用 Key"}, "fetch_models": map[string]any{"type": "boolean", "description": "Return candidate_models from /models without saving"}}, "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_delete",
			FullName:    builtinToolDeleteModelProvider,
			Title:       "删除模型供应商",
			Description: "按 id 删除全局模型供应商。删除前必须确认用户明确要求删除；正在被全局配置使用或最后一个供应商不能删除。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "要删除的供应商 id"}}, "required": []string{"id"}},
		},
	}
}

func isBuiltinModelProviderTool(name string) bool {
	switch name {
	case builtinToolListModelProviders, builtinToolSaveModelProvider, builtinToolDeleteModelProvider, builtinToolTestModelProvider:
		return true
	default:
		return false
	}
}

func (a *Server) callBuiltinModelProviderTool(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case builtinToolListModelProviders:
		return a.builtinListModelProviders(args)
	case builtinToolSaveModelProvider:
		return a.builtinSaveModelProvider(args)
	case builtinToolDeleteModelProvider:
		id, err := requiredStringArg(args, "id")
		if err != nil {
			return nil, err
		}
		if err := a.store.DeleteModelProvider(id); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "deleted_id": id}, nil
	case builtinToolTestModelProvider:
		return a.builtinTestModelProvider(ctx, args)
	default:
		return nil, fmt.Errorf("unknown builtin model provider tool: %s", name)
	}
}

func (a *Server) builtinSaveModelProvider(args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringArg(args, "id"))
	var previous *store.ModelProvider
	if id != "" {
		found, ok, err := a.findModelProvider(id)
		if err != nil {
			return nil, err
		}
		if ok {
			previous = &found
		}
	}
	input, err := modelProviderInputFromArgs(args, previous)
	if err != nil {
		return nil, err
	}
	setDefault, _ := optionalBoolArg(args, "set_as_global_default")
	globalModel := strings.TrimSpace(stringArg(args, "global_model"))
	provider, savedConfig, err := a.store.UpsertModelProvider(id, input, setDefault, globalModel)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"ok": true, "provider": provider, "secret_handling": "api_keys 已保存但不会回显；结果只包含 has_api_key/api_key_masked。"}
	if savedConfig != nil {
		result["global"] = map[string]any{"ok": true, "provider_id": savedConfig.ProviderID, "model": savedConfig.Model}
	}
	return result, nil
}

func (a *Server) builtinTestModelProvider(ctx context.Context, args map[string]any) (map[string]any, error) {
	id, err := requiredStringArg(args, "id")
	if err != nil {
		return nil, err
	}
	selectedKeyID := strings.TrimSpace(stringArg(args, "selected_key_id"))
	allKeys, _ := optionalBoolArg(args, "all_keys")
	fetchModels, _ := optionalBoolArg(args, "fetch_models")
	modelName := strings.TrimSpace(stringArg(args, "model"))
	keyConfigs, provider, err := a.store.ModelProviderKeyConfigs(id, selectedKeyID, modelName, allKeys)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	keyResults := make([]map[string]any, 0, len(keyConfigs))
	var savedProvider *store.ModelProvider
	selectedSuccess := false
	okAny := false
	for _, keyConfig := range keyConfigs {
		result, chatErr := a.testModelProviderKey(ctx, keyConfig, fetchModels)
		keyResults = append(keyResults, result)
		if keyConfig.KeyID == "" {
			okAny = okAny || chatErr == nil
			continue
		}
		if chatErr != nil {
			if _, err := a.store.MarkModelProviderKeyTestResult(provider.ID, keyConfig.KeyID, false, chatErr.Error(), false); err != nil {
				return nil, err
			}
			continue
		}
		okAny = true
		selectThis := !selectedSuccess
		updated, err := a.store.MarkModelProviderKeyTestResult(provider.ID, keyConfig.KeyID, true, "", selectThis)
		if err != nil {
			return nil, err
		}
		if selectThis {
			selectedSuccess = true
			savedProvider = &updated
		}
	}
	return modelProviderTestResponse(provider, modelName, fetchModels, okAny, keyResults, savedProvider), nil
}

func (a *Server) testModelProviderKey(ctx context.Context, keyConfig store.ModelProviderKeyConfig, fetchModels bool) (map[string]any, error) {
	cfg := keyConfig.Config
	cfg.SystemPrompt = "你是 ChatDock 的模型供应商连通性测试助手。请只回复 OK。"
	result := map[string]any{"key_id": keyConfig.KeyID, "key_name": keyConfig.KeyName, "model": cfg.Model}
	if fetchModels {
		models, err := a.client.ListModels(ctx, cfg)
		if err != nil {
			result["model_list_ok"] = false
			result["model_list_error"] = err.Error()
		} else {
			result["model_list_ok"] = true
			result["candidate_models"] = models
			result["models"] = models
			result["count"] = len(models)
			result["note"] = "model catalog only; add models manually to provider.models"
		}
	}
	chatErr := a.client.TestModelProvider(ctx, cfg)
	result["ok"] = chatErr == nil
	result["chat_test_ok"] = chatErr == nil
	if chatErr != nil {
		result["chat_test_error"] = chatErr.Error()
		result["error"] = chatErr.Error()
	}
	return result, chatErr
}

func modelProviderTestResponse(provider store.ModelProvider, modelName string, fetchModels bool, okAny bool, keyResults []map[string]any, savedProvider *store.ModelProvider) map[string]any {
	operation := "chat_test"
	if fetchModels {
		operation = "chat_test_with_model_list"
	}
	response := map[string]any{"ok": okAny, "operation": operation, "provider_id": provider.ID, "model": modelName, "key_results": keyResults}
	if modelName == "" {
		response["model"] = provider.DefaultModel
	}
	if len(keyResults) == 1 {
		for key, value := range keyResults[0] {
			if key != "key_id" && key != "key_name" {
				response[key] = value
			}
		}
		response["selected_key_id"] = keyResults[0]["key_id"]
	}
	if savedProvider != nil {
		response["provider"] = *savedProvider
	}
	return response
}

func (a *Server) builtinListModelProviders(args map[string]any) (map[string]any, error) {
	providers, err := a.store.ListModelProviders()
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(stringArg(args, "id"))
	query := strings.ToLower(strings.TrimSpace(stringArg(args, "query")))
	filtered := make([]store.ModelProvider, 0, len(providers))
	for _, provider := range providers {
		if id != "" && provider.ID != id {
			continue
		}
		if query != "" {
			text := strings.ToLower(strings.Join([]string{provider.ID, provider.Name, provider.BaseURL, provider.DefaultModel, strings.Join(provider.Models, " ")}, " "))
			if !strings.Contains(text, query) {
				continue
			}
		}
		filtered = append(filtered, provider)
	}
	return map[string]any{"providers": filtered, "count": len(filtered), "secret_handling": "结果只包含 has_api_key/api_key_masked，不包含明文 api_key。"}, nil
}

func (a *Server) findModelProvider(id string) (store.ModelProvider, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.ModelProvider{}, false, fmt.Errorf("model provider id is empty")
	}
	providers, err := a.store.ListModelProviders()
	if err != nil {
		return store.ModelProvider{}, false, err
	}
	for _, provider := range providers {
		if provider.ID == id || strings.EqualFold(provider.ID, id) {
			return provider, true, nil
		}
	}
	return store.ModelProvider{}, false, nil
}

func modelProviderInputFromArgs(args map[string]any, previous *store.ModelProvider) (store.ModelProviderInput, error) {
	enabled := true
	input := store.ModelProviderInput{Type: "openai-compatible", Enabled: &enabled, TimeoutMS: 120000, KeyStrategy: "auto"}
	if previous != nil {
		input = modelProviderInputFromProvider(*previous)
	}
	if value, ok := optionalStringArg(args, "id"); ok {
		input.ID = value
	}
	if value, ok := optionalStringArg(args, "name"); ok {
		input.Name = value
	}
	if value, ok := optionalStringArg(args, "base_url"); ok {
		input.BaseURL = value
	}
	if value, ok := optionalStringArg(args, "default_model"); ok {
		input.DefaultModel = value
	}
	if models, ok, err := optionalStringSliceArg(args, "models"); err != nil {
		return store.ModelProviderInput{}, err
	} else if ok {
		input.Models = models
	}
	if value, ok, err := optionalIntArg(args, "timeout_ms"); err != nil {
		return store.ModelProviderInput{}, err
	} else if ok {
		input.TimeoutMS = value
	}
	if value, ok := optionalBoolArg(args, "enabled"); ok {
		input.Enabled = &value
	}
	if value, ok := optionalStringArg(args, "key_strategy"); ok {
		input.KeyStrategy = strings.ToLower(value)
	}
	if value, ok := optionalStringArg(args, "selected_key_id"); ok {
		input.SelectedKeyID = value
	}
	if apiKeys, ok, err := optionalAPIKeyInputsArg(args, "api_keys"); err != nil {
		return store.ModelProviderInput{}, err
	} else if ok {
		input.APIKeys = apiKeys
	}
	if strings.TrimSpace(input.Name) == "" {
		return store.ModelProviderInput{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.BaseURL) == "" {
		return store.ModelProviderInput{}, fmt.Errorf("base_url is required")
	}
	if strings.TrimSpace(input.DefaultModel) == "" {
		return store.ModelProviderInput{}, fmt.Errorf("default_model is required")
	}
	return input, nil
}

func modelProviderInputFromProvider(provider store.ModelProvider) store.ModelProviderInput {
	keys := make([]store.ModelProviderAPIKeyInput, 0, len(provider.APIKeys))
	enabled := provider.Enabled
	for _, key := range provider.APIKeys {
		enabled := key.Enabled
		keys = append(keys, store.ModelProviderAPIKeyInput{ID: key.ID, Name: key.Name, APIKey: "********", Enabled: &enabled, Priority: key.Priority})
	}
	return store.ModelProviderInput{
		ID:            provider.ID,
		Name:          provider.Name,
		Type:          provider.Type,
		BaseURL:       provider.BaseURL,
		DefaultModel:  provider.DefaultModel,
		Models:        append([]string(nil), provider.Models...),
		TimeoutMS:     provider.TimeoutMS,
		Enabled:       &enabled,
		KeyStrategy:   provider.KeyStrategy,
		SelectedKeyID: provider.SelectedKeyID,
		APIKeys:       keys,
	}
}

func optionalStringSliceArg(args map[string]any, key string) ([]string, bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, false, nil
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	switch items := value.(type) {
	case []any:
		for index, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, true, fmt.Errorf("%s[%d] must be string", key, index)
			}
			add(text)
		}
	case []string:
		for _, item := range items {
			add(item)
		}
	default:
		return nil, true, fmt.Errorf("%s must be array of strings", key)
	}
	return out, true, nil
}

func optionalAPIKeyInputsArg(args map[string]any, key string) ([]store.ModelProviderAPIKeyInput, bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, false, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, true, fmt.Errorf("%s must be array", key)
	}
	out := make([]store.ModelProviderAPIKeyInput, 0, len(items))
	for idx, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("%s[%d] must be object", key, idx)
		}
		input := store.ModelProviderAPIKeyInput{}
		if value, ok := optionalStringArg(raw, "id"); ok {
			input.ID = value
		}
		if value, ok := optionalStringArg(raw, "name"); ok {
			input.Name = value
		}
		if value, ok := optionalStringArg(raw, "api_key"); ok {
			input.APIKey = value
		}
		if enabled, ok := optionalBoolArg(raw, "enabled"); ok {
			input.Enabled = &enabled
		}
		if priority, ok, err := optionalIntArg(raw, "priority"); err != nil {
			return nil, true, err
		} else if ok {
			input.Priority = priority
		}
		out = append(out, input)
	}
	return out, true, nil
}
