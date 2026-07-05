package chatdock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	"chatdock/internal/chatdock/store"
)

const (
	builtinToolListModelProviders        = "chatdock_model_providers_list"
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
	providerProps := map[string]any{
		"name":          map[string]any{"type": "string", "description": "供应商显示名称，例如 OpenAI、Volc Ark、Ollama、LM Studio"},
		"base_url":      map[string]any{"type": "string", "description": "OpenAI 兼容 Base URL，例如 https://api.openai.com/v1 或 http://127.0.0.1:11434/v1"},
		"api_key":       map[string]any{"type": "string", "description": "API Key。创建或修改时可传；留空或传 ******** 表示保留原 Key。工具结果不会回显明文 Key。"},
		"default_model": map[string]any{"type": "string", "description": "默认模型名称，例如 gpt-4o-mini、doubao-seed-1-6、qwen3-coder"},
		"models":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "可选模型列表。也兼容传入逗号或换行分隔字符串。"},
		"timeout_ms":    map[string]any{"type": "integer", "description": "请求超时毫秒，默认 120000"},
		"enabled":       map[string]any{"type": "boolean", "description": "是否启用供应商，默认 true"},
	}
	return []mcp.MCPTool{
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_providers_list",
			FullName:    builtinToolListModelProviders,
			Title:       "查询模型供应商",
			Description: "查询全局模型供应商。可传 id 精确查询，或传 query 按名称、Base URL、模型名模糊过滤。结果只返回 has_api_key 和掩码，不返回明文 API Key。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "query": map[string]any{"type": "string"}}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_create",
			FullName:    builtinToolCreateModelProvider,
			Title:       "新增模型供应商",
			Description: "新增一个全局 OpenAI 兼容模型供应商。适合用户提供 Base URL、API Key、默认模型后使用。创建后不会自动切换当前工作空间，若要切换请再调用 chatdock_workspace_model_provider_set。",
			InputSchema: map[string]any{"type": "object", "properties": mergeSchemaProps(map[string]any{"id": map[string]any{"type": "string", "description": "可选供应商 id；为空时自动按名称或域名生成"}}, providerProps), "required": []string{"name", "base_url", "default_model"}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_update",
			FullName:    builtinToolUpdateModelProvider,
			Title:       "编辑模型供应商",
			Description: "按 id 编辑全局模型供应商。未传字段会保留原值；api_key 留空或传 ******** 会保留原 Key。",
			InputSchema: map[string]any{"type": "object", "properties": mergeSchemaProps(map[string]any{"id": map[string]any{"type": "string", "description": "要编辑的供应商 id"}}, providerProps), "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_set_enabled",
			FullName:    builtinToolSetModelProviderEnabled,
			Title:       "启用或停用模型供应商",
			Description: "按 id 启用或停用全局模型供应商。启用不会自动切换当前工作空间；需要切换默认供应商时调用 chatdock_workspace_model_provider_set。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "供应商 id"}, "enabled": map[string]any{"type": "boolean", "description": "true=启用，false=停用"}}, "required": []string{"id", "enabled"}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_delete",
			FullName:    builtinToolDeleteModelProvider,
			Title:       "删除模型供应商",
			Description: "按 id 删除全局模型供应商。删除前必须确认用户明确要求删除；正在被工作空间使用或最后一个供应商不能删除。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "要删除的供应商 id"}}, "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_test",
			FullName:    builtinToolTestModelProvider,
			Title:       "测试模型供应商",
			Description: "按 id 测试全局模型供应商的 chat/completions 连接。可传 model 覆盖默认模型。不会回显 API Key。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "供应商 id"}, "model": map[string]any{"type": "string", "description": "可选，临时测试的模型名"}}, "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "model_provider_models",
			FullName:    builtinToolListModelProviderModels,
			Title:       "获取供应商模型列表",
			Description: "按 id 请求供应商 /models 接口。save=true 时会把返回模型列表保存到该供应商。不会回显 API Key。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "供应商 id"}, "save": map[string]any{"type": "boolean", "description": "是否保存返回的模型列表，默认 false"}}, "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerModelProviders,
			Name:        "workspace_model_provider_set",
			FullName:    builtinToolSetWorkspaceModelProvider,
			Title:       "设置当前工作空间默认供应商",
			Description: "把当前工作空间默认模型切换到指定全局供应商和模型。只影响当前工作空间，不会修改供应商连接信息。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"provider_id": map[string]any{"type": "string", "description": "全局供应商 id"}, "model": map[string]any{"type": "string", "description": "可选模型名；为空时使用供应商默认模型"}}, "required": []string{"provider_id"}},
		},
	}
}

func isBuiltinModelProviderTool(name string) bool {
	switch name {
	case builtinToolListModelProviders, builtinToolCreateModelProvider, builtinToolUpdateModelProvider, builtinToolSetModelProviderEnabled, builtinToolDeleteModelProvider, builtinToolTestModelProvider, builtinToolListModelProviderModels, builtinToolSetWorkspaceModelProvider:
		return true
	default:
		return false
	}
}

func (a *App) callBuiltinModelProviderTool(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case builtinToolListModelProviders:
		return a.builtinListModelProviders(args)
	case builtinToolCreateModelProvider:
		input, err := modelProviderInputFromArgs(args, nil)
		if err != nil {
			return nil, err
		}
		provider, err := a.store.CreateModelProvider(input)
		if err != nil {
			return nil, err
		}
		if !input.Enabled {
			disabledInput := modelProviderInputFromProvider(provider)
			disabledInput.Enabled = false
			provider, err = a.store.UpdateModelProvider(provider.ID, disabledInput)
			if err != nil {
				return nil, err
			}
		}
		return map[string]any{"ok": true, "provider": provider, "secret_handling": "api_key 已保存但不会回显。"}, nil
	case builtinToolUpdateModelProvider:
		id, err := requiredStringArg(args, "id")
		if err != nil {
			return nil, err
		}
		previous, err := a.findModelProvider(id)
		if err != nil {
			return nil, err
		}
		input, err := modelProviderInputFromArgs(args, &previous)
		if err != nil {
			return nil, err
		}
		provider, err := a.store.UpdateModelProvider(id, input)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "provider": provider, "secret_handling": "api_key 已保存但不会回显。"}, nil
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
		provider, err := a.store.UpdateModelProvider(id, input)
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
		cfg, provider, err := a.modelProviderConfigFromArgs(args)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := a.client.TestModelProvider(ctx, cfg); err != nil {
			return map[string]any{"ok": false, "provider_id": provider.ID, "model": cfg.Model, "error": err.Error()}, nil
		}
		return map[string]any{"ok": true, "provider_id": provider.ID, "model": cfg.Model}, nil
	case builtinToolListModelProviderModels:
		cfg, provider, err := a.modelProviderConfigFromArgs(args)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		models, err := a.client.ListModels(ctx, cfg)
		if err != nil {
			return map[string]any{"ok": false, "provider_id": provider.ID, "error": err.Error(), "models": []string{}}, nil
		}
		result := map[string]any{"ok": true, "provider_id": provider.ID, "models": models, "count": len(models)}
		if save, ok := optionalBoolArg(args, "save"); ok && save {
			input := modelProviderInputFromProvider(provider)
			input.Models = models
			if input.DefaultModel == "" && len(models) > 0 {
				input.DefaultModel = models[0]
			}
			updated, err := a.store.UpdateModelProvider(provider.ID, input)
			if err != nil {
				return nil, err
			}
			result["saved"] = true
			result["provider"] = updated
		}
		return result, nil
	case builtinToolSetWorkspaceModelProvider:
		providerID, err := requiredStringArg(args, "provider_id")
		if err != nil {
			return nil, err
		}
		provider, err := a.findModelProvider(providerID)
		if err != nil {
			return nil, err
		}
		if !provider.Enabled {
			return nil, fmt.Errorf("model provider is disabled: %s", provider.ID)
		}
		cfg := a.store.GetModelConfig()
		cfg.ProviderID = provider.ID
		modelName := strings.TrimSpace(stringArg(args, "model"))
		if modelName == "" {
			modelName = provider.DefaultModel
		}
		cfg.Model = modelName
		cfg.Models = append([]string(nil), provider.Models...)
		saved, err := a.store.SaveModelConfig(cfg)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "workspace": a.store.ActivePrompt(), "provider_id": saved.ProviderID, "model": saved.Model}, nil
	default:
		return nil, fmt.Errorf("unknown builtin model provider tool: %s", name)
	}
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

func (a *App) modelProviderConfigFromArgs(args map[string]any) (model.ModelConfig, store.ModelProvider, error) {
	id, err := requiredStringArg(args, "id")
	if err != nil {
		return model.ModelConfig{}, store.ModelProvider{}, err
	}
	provider, err := a.findModelProvider(id)
	if err != nil {
		return model.ModelConfig{}, store.ModelProvider{}, err
	}
	cfg, ok, err := a.store.ModelProviderConfig(provider.ID)
	if err != nil {
		return model.ModelConfig{}, store.ModelProvider{}, err
	}
	if !ok {
		return model.ModelConfig{}, store.ModelProvider{}, fmt.Errorf("model provider not found: %s", provider.ID)
	}
	if modelName := strings.TrimSpace(stringArg(args, "model")); modelName != "" {
		cfg.Model = modelName
	}
	cfg.SystemPrompt = "你是 ChatDock 的模型供应商连通性测试助手。请只回复 OK。"
	return model.NormalizeModelConfig(cfg), provider, nil
}

func modelProviderInputFromArgs(args map[string]any, previous *store.ModelProvider) (store.ModelProviderInput, error) {
	input := store.ModelProviderInput{Type: "openai-compatible", Enabled: true, TimeoutMS: 120000}
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
	return store.ModelProviderInput{
		ID:           provider.ID,
		Name:         provider.Name,
		Type:         provider.Type,
		BaseURL:      provider.BaseURL,
		APIKey:       "********",
		DefaultModel: provider.DefaultModel,
		Models:       append([]string(nil), provider.Models...),
		TimeoutMS:    provider.TimeoutMS,
		Enabled:      provider.Enabled,
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
