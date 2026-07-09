package chatdock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/store"
)

const (
	builtinToolListModelProviders        = "chatdock_model_providers_list"
	builtinToolSaveModelProvider         = "chatdock_model_provider_save"
	builtinToolCreateModelProvider       = "chatdock_model_provider_create"
	builtinToolUpdateModelProvider       = "chatdock_model_provider_update"
	builtinToolSetModelProviderEnabled   = "chatdock_model_provider_set_enabled"
	builtinToolDeleteModelProvider       = "chatdock_model_provider_delete"
	builtinToolTestModelProvider         = "chatdock_model_provider_test"
	builtinToolListModelProviderModels   = "chatdock_model_provider_models"
	builtinToolSetWorkspaceModelProvider = "chatdock_workspace_model_provider_set"
	builtinToolServerModelProviders      = "chatdock"
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
		"id":                       map[string]any{"type": "string", "description": "供应商 id；存在则编辑，不存在则创建；为空时创建并自动生成"},
		"name":                     map[string]any{"type": "string", "description": "供应商显示名称，例如 OpenAI、Volc Ark、Ollama、LM Studio"},
		"base_url":                 map[string]any{"type": "string", "description": "OpenAI 兼容 Base URL，例如 https://api.openai.com/v1 或 http://127.0.0.1:11434/v1"},
		"api_key":                  map[string]any{"type": "string", "description": "兼容旧单 Key 参数。传入后会写入/更新当前 Key；留空或 ******** 表示保留。推荐改用 api_keys。"},
		"default_model":            map[string]any{"type": "string", "description": "默认模型名称，例如 gpt-4o-mini、doubao-seed-1-6、qwen3-coder"},
		"models":                   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "可用模型列表，必须由用户手动添加或确认；也兼容传入逗号或换行分隔字符串。"},
		"timeout_ms":               map[string]any{"type": "integer", "description": "请求超时毫秒，默认 120000"},
		"enabled":                  map[string]any{"type": "boolean", "description": "是否启用供应商，默认 true"},
		"key_strategy":             map[string]any{"type": "string", "enum": []string{"auto", "manual"}, "description": "Key 选择策略。auto=优先 selected_key_id，失败/不可用时按 priority 选择其他启用 Key；manual=只使用 selected_key_id。默认 auto。"},
		"selected_key_id":          map[string]any{"type": "string", "description": "当前选中的 Key id。manual 策略必须有效；auto 策略下会优先使用它。"},
		"api_keys":                 map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": keyProps}, "description": "同一供应商的多个 Key。结果只返回 has_api_key/api_key_masked 和测试状态，不返回明文。"},
		"set_as_workspace_default": map[string]any{"type": "boolean", "description": "保存后是否设为当前工作空间默认供应商"},
		"workspace_model":          map[string]any{"type": "string", "description": "设为当前工作空间默认供应商时使用的模型；为空用 default_model"},
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
			Description: "新增或编辑 OpenAI 兼容模型供应商；也可启用/停用、设置当前工作空间默认供应商，并在 api_keys 中维护多个 Key。",
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
			Description: "按 id 删除全局模型供应商。删除前必须确认用户明确要求删除；正在被工作空间使用或最后一个供应商不能删除。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "要删除的供应商 id"}}, "required": []string{"id"}},
		},
	}
}

func isBuiltinModelProviderTool(name string) bool {
	switch name {
	case builtinToolListModelProviders, builtinToolSaveModelProvider, builtinToolCreateModelProvider, builtinToolUpdateModelProvider, builtinToolSetModelProviderEnabled, builtinToolDeleteModelProvider, builtinToolTestModelProvider, builtinToolListModelProviderModels, builtinToolSetWorkspaceModelProvider:
		return true
	default:
		return false
	}
}

func (a *App) callBuiltinModelProviderTool(ctx context.Context, workspaceID string, name string, args map[string]any) (any, error) {
	switch name {
	case builtinToolListModelProviders:
		return a.builtinListModelProviders(args)
	case builtinToolSaveModelProvider:
		return a.builtinSaveModelProvider(workspaceID, args)
	case builtinToolCreateModelProvider:
		return a.builtinSaveModelProvider(workspaceID, args)
	case builtinToolUpdateModelProvider:
		if _, ok := args["id"]; !ok {
			return nil, fmt.Errorf("id is required")
		}
		return a.builtinSaveModelProvider(workspaceID, args)
	case builtinToolSetModelProviderEnabled:
		id, err := requiredStringArg(args, "id")
		if err != nil {
			return nil, err
		}
		enabled, ok := optionalBoolArg(args, "enabled")
		if !ok {
			return nil, fmt.Errorf("enabled is required and must be boolean")
		}
		previous, err := a.findModelProvider(id)
		if err != nil {
			return nil, err
		}
		input := modelProviderInputFromProvider(previous)
		input.Enabled = enabled
		provider, err := a.store.UpdateModelProvider(previous.ID, input)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "provider": provider}, nil
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
	case builtinToolListModelProviderModels:
		return a.builtinListModelProviderModels(ctx, args)
	case builtinToolSetWorkspaceModelProvider:
		providerID, err := requiredStringArg(args, "provider_id")
		if err != nil {
			return nil, err
		}
		workspace, err := a.setWorkspaceModelProvider(workspaceID, providerID, strings.TrimSpace(stringArg(args, "model")))
		if err != nil {
			return nil, err
		}
		return workspace, nil
	default:
		return nil, fmt.Errorf("unknown builtin model provider tool: %s", name)
	}
}

func (a *App) builtinSaveModelProvider(workspaceID string, args map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(stringArg(args, "id"))
	var previous *store.ModelProvider
	if id != "" {
		if found, err := a.findModelProvider(id); err == nil {
			previous = &found
		}
	}
	input, err := modelProviderInputFromArgs(args, previous)
	if err != nil {
		return nil, err
	}
	var provider store.ModelProvider
	if previous != nil {
		provider, err = a.store.UpdateModelProvider(previous.ID, input)
	} else {
		provider, err = a.store.CreateModelProvider(input)
		if err == nil && !input.Enabled {
			disabledInput := modelProviderInputFromProvider(provider)
			disabledInput.Enabled = false
			provider, err = a.store.UpdateModelProvider(provider.ID, disabledInput)
		}
	}
	if err != nil {
		return nil, err
	}
	result := map[string]any{"ok": true, "provider": provider, "secret_handling": "api_key/api_keys 已保存但不会回显；结果只包含 has_api_key/api_key_masked。"}
	if setDefault, ok := optionalBoolArg(args, "set_as_workspace_default"); ok && setDefault {
		workspaceModel := strings.TrimSpace(stringArg(args, "workspace_model"))
		workspace, err := a.setWorkspaceModelProvider(workspaceID, provider.ID, workspaceModel)
		if err != nil {
			return nil, err
		}
		result["workspace"] = workspace
	}
	return result, nil
}

func (a *App) builtinListModelProviderModels(ctx context.Context, args map[string]any) (map[string]any, error) {
	id, err := requiredStringArg(args, "id")
	if err != nil {
		return nil, err
	}
	selectedKeyID := strings.TrimSpace(stringArg(args, "selected_key_id"))
	allKeys, _ := optionalBoolArg(args, "all_keys")
	modelName := strings.TrimSpace(stringArg(args, "model"))
	keyConfigs, provider, err := a.store.ModelProviderKeyConfigs(id, selectedKeyID, modelName, allKeys)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	keyResults := make([]map[string]any, 0, len(keyConfigs))
	okAny := false
	for _, item := range keyConfigs {
		cfg := item.Config
		result := map[string]any{"key_id": item.KeyID, "key_name": item.KeyName, "model": cfg.Model, "operation": "model_catalog"}
		models, listErr := a.client.ListModels(ctx, cfg)
		if listErr != nil {
			result["ok"] = false
			result["model_list_ok"] = false
			result["error"] = listErr.Error()
			keyResults = append(keyResults, result)
			continue
		}
		result["ok"] = true
		result["model_list_ok"] = true
		result["candidate_models"] = models
		result["models"] = models
		result["count"] = len(models)
		result["note"] = "model catalog only; add models manually to provider.models"
		okAny = true
		keyResults = append(keyResults, result)
	}
	response := map[string]any{"ok": okAny, "operation": "model_catalog", "provider_id": provider.ID, "model": modelName, "key_results": keyResults, "note": "candidate model catalog only; add usable models manually to provider.models"}
	if modelName == "" {
		response["model"] = provider.DefaultModel
	}
	if len(keyResults) == 1 {
		for k, v := range keyResults[0] {
			if k != "key_id" && k != "key_name" {
				response[k] = v
			}
		}
		response["selected_key_id"] = keyResults[0]["key_id"]
	}
	return response, nil
}

func (a *App) builtinTestModelProvider(ctx context.Context, args map[string]any) (map[string]any, error) {
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
	okAny := false
	selectedSuccess := false
	for _, item := range keyConfigs {
		cfg := item.Config
		cfg.SystemPrompt = "你是 ChatDock 的模型供应商连通性测试助手。请只回复 OK。"
		result := map[string]any{"key_id": item.KeyID, "key_name": item.KeyName, "model": cfg.Model}
		var models []string
		var listErr error
		if fetchModels {
			models, listErr = a.client.ListModels(ctx, cfg)
			if listErr == nil {
				result["model_list_ok"] = true
				result["candidate_models"] = models
				result["models"] = models
				result["count"] = len(models)
				result["note"] = "model catalog only; add models manually to provider.models"
			} else {
				result["model_list_ok"] = false
				result["model_list_error"] = listErr.Error()
			}
		}

		chatErr := a.client.TestModelProvider(ctx, cfg)
		if chatErr != nil {
			result["ok"] = false
			result["chat_test_ok"] = false
			result["chat_test_error"] = chatErr.Error()
			result["error"] = chatErr.Error()
			if item.KeyID != "" {
				_, _ = a.store.MarkModelProviderKeyTestResult(provider.ID, item.KeyID, false, chatErr.Error(), false)
			}
			keyResults = append(keyResults, result)
			continue
		}

		result["ok"] = true
		result["chat_test_ok"] = true
		okAny = true
		if item.KeyID != "" {
			selectThis := !selectedSuccess
			updated, _ := a.store.MarkModelProviderKeyTestResult(provider.ID, item.KeyID, true, "", selectThis)
			if selectThis {
				selectedSuccess = true
				savedProvider = &updated
			}
		}

		keyResults = append(keyResults, result)
	}
	operation := "chat_test"
	if fetchModels {
		operation = "chat_test_with_model_list"
	}
	response := map[string]any{"ok": okAny, "operation": operation, "provider_id": provider.ID, "model": modelName, "key_results": keyResults}
	if modelName == "" {
		response["model"] = provider.DefaultModel
	}
	if len(keyResults) == 1 {
		for k, v := range keyResults[0] {
			if k != "key_id" && k != "key_name" {
				response[k] = v
			}
		}
		response["selected_key_id"] = keyResults[0]["key_id"]
	}
	if savedProvider != nil {
		response["provider"] = *savedProvider
	}
	return response, nil
}

func (a *App) builtinListModelProviders(args map[string]any) (map[string]any, error) {
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

func (a *App) findModelProvider(id string) (store.ModelProvider, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.ModelProvider{}, fmt.Errorf("model provider id is empty")
	}
	providers, err := a.store.ListModelProviders()
	if err != nil {
		return store.ModelProvider{}, err
	}
	for _, provider := range providers {
		if provider.ID == id || strings.EqualFold(provider.ID, id) {
			return provider, nil
		}
	}
	return store.ModelProvider{}, fmt.Errorf("model provider not found: %s", id)
}

func (a *App) setWorkspaceModelProvider(workspaceID string, providerID string, modelName string) (map[string]any, error) {
	provider, err := a.findModelProvider(providerID)
	if err != nil {
		return nil, err
	}
	if !provider.Enabled {
		return nil, fmt.Errorf("model provider is disabled: %s", provider.ID)
	}
	cfg := a.store.GetModelConfig(workspaceID)
	cfg.ProviderID = provider.ID
	if strings.TrimSpace(modelName) == "" {
		modelName = provider.DefaultModel
	}
	cfg.Model = strings.TrimSpace(modelName)
	cfg.Models = append([]string(nil), provider.Models...)
	saved, err := a.store.SaveModelConfig(workspaceID, cfg)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "workspace": workspaceID, "provider_id": saved.ProviderID, "model": saved.Model}, nil
}

func modelProviderInputFromArgs(args map[string]any, previous *store.ModelProvider) (store.ModelProviderInput, error) {
	input := store.ModelProviderInput{Type: "openai-compatible", Enabled: true, TimeoutMS: 120000, KeyStrategy: "auto"}
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
	if value, ok := optionalStringArg(args, "api_key"); ok {
		input.APIKey = value
	}
	if value, ok := optionalStringArg(args, "default_model"); ok {
		input.DefaultModel = value
	}
	if models, ok := optionalStringSliceArg(args, "models"); ok {
		input.Models = models
	}
	if value, ok, err := optionalIntArg(args, "timeout_ms"); err != nil {
		return store.ModelProviderInput{}, err
	} else if ok {
		input.TimeoutMS = value
	}
	if value, ok := optionalBoolArg(args, "enabled"); ok {
		input.Enabled = value
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
	for _, key := range provider.APIKeys {
		enabled := key.Enabled
		keys = append(keys, store.ModelProviderAPIKeyInput{ID: key.ID, Name: key.Name, APIKey: "********", Enabled: &enabled, Priority: key.Priority})
	}
	return store.ModelProviderInput{
		ID:            provider.ID,
		Name:          provider.Name,
		Type:          provider.Type,
		BaseURL:       provider.BaseURL,
		APIKey:        "********",
		DefaultModel:  provider.DefaultModel,
		Models:        append([]string(nil), provider.Models...),
		TimeoutMS:     provider.TimeoutMS,
		Enabled:       provider.Enabled,
		KeyStrategy:   provider.KeyStrategy,
		SelectedKeyID: provider.SelectedKeyID,
		APIKeys:       keys,
	}
}

func optionalStringSliceArg(args map[string]any, key string) ([]string, bool) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, false
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
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			add(fmt.Sprint(item))
		}
	case []string:
		for _, item := range v {
			add(item)
		}
	case string:
		for _, item := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '，' || r == '\n' || r == '\t' }) {
			add(item)
		}
	default:
		add(fmt.Sprint(v))
	}
	return out, true
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
