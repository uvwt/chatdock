package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *ChatClient) Embed(ctx context.Context, baseURL string, apiKey string, model string, inputs []string) ([][]float64, error) {
	baseURL = strings.TrimSpace(baseURL)
	model = strings.TrimSpace(model)
	if baseURL == "" {
		return nil, fmt.Errorf("embedding base url is empty")
	}
	if model == "" {
		model = "BAAI/bge-m3"
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/embeddings"
	body := map[string]any{"model": model, "input": inputs}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding api failed: %s: %s", resp.Status, summarizeModelProviderBody(resp.Header.Get("Content-Type"), respBody))
	}
	var output struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &output); err != nil {
		return nil, err
	}
	if len(output.Data) != len(inputs) {
		return nil, fmt.Errorf("embedding result count mismatch: got %d want %d", len(output.Data), len(inputs))
	}
	out := make([][]float64, len(inputs))
	for i, item := range output.Data {
		index := item.Index
		if index < 0 || index >= len(inputs) {
			index = i
		}
		out[index] = item.Embedding
	}
	for i := range out {
		if len(out[i]) == 0 {
			return nil, fmt.Errorf("embedding result %d is empty", i)
		}
	}
	return out, nil
}
