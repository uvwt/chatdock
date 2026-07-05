package chatdock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"chatdock/internal/chatdock/model"
	"chatdock/internal/chatdock/store"
)

func (a *App) handleListModelProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := a.store.ListModelProviders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"providers": providers})
}

func (a *App) handleCreateModelProvider(w http.ResponseWriter, r *http.Request) {
	var input store.ModelProviderInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	provider, err := a.store.CreateModelProvider(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, provider)
}

func (a *App) handleModelProviderRoute(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/model-providers/"), "/")
	if id == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("model provider route not found"))
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input store.ModelProviderInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		provider, err := a.store.UpdateModelProvider(id, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, provider)
	case http.MethodDelete:
		if err := a.store.DeleteModelProvider(id); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (a *App) modelConfigFromRequest(r *http.Request) (model.ModelConfig, error) {
	cfg := a.store.GetModelConfig()
	if r.Body == nil {
		return cfg, nil
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		return model.ModelConfig{}, err
	}
	if strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "{}" {
		return cfg, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return model.ModelConfig{}, err
	}
	next := cfg
	if err := json.Unmarshal(raw, &next); err != nil {
		return model.ModelConfig{}, err
	}
	resolvedFromProvider := false
	if _, hasProviderID := fields["provider_id"]; hasProviderID && strings.TrimSpace(next.ProviderID) != "" {
		if providerCfg, ok, err := a.store.ModelProviderConfig(next.ProviderID); err != nil {
			return model.ModelConfig{}, err
		} else if ok {
			resolvedFromProvider = true
			_, hasBaseURL := fields["base_url"]
			_, hasModel := fields["model"]
			_, hasModels := fields["models"]
			_, hasAPIKey := fields["api_key"]
			if !hasBaseURL || strings.TrimSpace(next.BaseURL) == "" {
				next.BaseURL = providerCfg.BaseURL
			}
			if !hasModel || strings.TrimSpace(next.Model) == "" {
				next.Model = providerCfg.Model
			}
			if !hasModels || len(next.Models) == 0 {
				next.Models = append([]string(nil), providerCfg.Models...)
			}
			if !hasAPIKey || strings.TrimSpace(next.APIKey) == "" || isMaskedModelSecret(next.APIKey) {
				next.APIKey = providerCfg.APIKey
			}
		} else {
			return model.ModelConfig{}, fmt.Errorf("model provider not found: %s", next.ProviderID)
		}
	}
	// 前端密码框为空或显示掩码时，继续复用已保存 Key；但如果本次按 provider_id 解析，
	// 只能使用该 provider 自己的 key，不能错误回退到当前工作区/其他 provider 的 key。
	if !resolvedFromProvider && (strings.TrimSpace(next.APIKey) == "" || isMaskedModelSecret(next.APIKey)) {
		next.APIKey = cfg.APIKey
	}
	return model.NormalizeModelConfig(next), nil
}

func isMaskedModelSecret(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	return strings.Contains(value, "*")
}

func (a *App) handleTestModelProvider(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.modelConfigFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.client.TestModelProvider(ctx, cfg); err != nil {
		writeJSONResponse(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "provider_id": cfg.ProviderID, "model": cfg.Model})
}

func (a *App) handleListProviderModels(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.modelConfigFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	models, err := a.client.ListModels(ctx, cfg)
	if err != nil {
		writeJSONResponse(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error(), "models": []string{}})
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "operation": "model_catalog", "provider_id": cfg.ProviderID, "candidate_models": models, "models": models, "note": "上游 /models 返回的是候选模型目录，不代表当前账号可用；不会自动写入供应商可用模型列表。"})
}
