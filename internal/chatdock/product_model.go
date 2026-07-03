package chatdock

import (
	"chatdock/internal/chatdock/model"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Store) ListModelProviders() ([]ModelProvider, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	providers := make([]ModelProvider, 0, len(workspaces.Workspaces))
	for _, ws := range workspaces.Workspaces {
		cfg, err := s.modelConfigForPrompt(ws.ID)
		if err != nil {
			return nil, err
		}
		providers = append(providers, ModelProvider{
			ID:            cfg.ProviderID,
			Name:          providerName(ws.Name, cfg),
			Type:          "openai-compatible",
			BaseURL:       cfg.BaseURL,
			HasAPIKey:     strings.TrimSpace(cfg.APIKey) != "",
			APIKeyMasked:  maskSecret(cfg.APIKey),
			DefaultModel:  cfg.Model,
			TimeoutMS:     120000,
			Enabled:       strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "",
			WorkspaceID:   ws.ID,
			WorkspaceName: ws.Name,
			CreatedAt:     ws.CreatedAt,
			UpdatedAt:     ws.UpdatedAt,
		})
	}
	return providers, nil
}

func (s *Store) modelConfigForPrompt(prompt string) (model.ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelConfigForPromptLocked(prompt)
}

func (s *Store) modelConfigForPromptLocked(prompt string) (model.ModelConfig, error) {
	raw, ok, err := s.getPromptRawLocked(prompt, "config")
	if err != nil {
		return model.ModelConfig{}, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return model.DefaultModelConfig(), nil
	}
	var cfg model.ModelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return model.ModelConfig{}, err
	}
	return model.NormalizeModelConfig(cfg), nil
}

func providerName(workspace string, cfg model.ModelConfig) string {
	base := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cfg.BaseURL), "https://"), "http://")
	if base == "" {
		base = "OpenAI Compatible"
	}
	return workspace + " · " + base
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "******"
	}
	return value[:4] + "******" + value[len(value)-4:]
}

func (a *App) handleListModelProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := a.store.ListModelProviders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"providers": providers})
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
	next := cfg
	if err := json.Unmarshal(raw, &next); err != nil {
		return model.ModelConfig{}, err
	}
	// 前端密码框为空或显示掩码时，继续复用已保存 Key；接口响应绝不回显 Key。
	if strings.TrimSpace(next.APIKey) == "" || strings.TrimSpace(next.APIKey) == "********" {
		next.APIKey = cfg.APIKey
	}
	return model.NormalizeModelConfig(next), nil
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
	writeJSONResponse(w, http.StatusOK, map[string]any{"ok": true, "provider_id": cfg.ProviderID, "models": models})
}
