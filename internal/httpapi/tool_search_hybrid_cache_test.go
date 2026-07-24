package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"chatdock/internal/llm"
	"chatdock/internal/model"
)

func newEmbeddingTestServer(t *testing.T, vector []float64) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": vector}}})
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func TestQueryEmbeddingCacheSeparatesBaseURLs(t *testing.T) {
	firstServer, firstRequests := newEmbeddingTestServer(t, []float64{1, 0})
	secondServer, secondRequests := newEmbeddingTestServer(t, []float64{0, 1})
	app := &Server{client: llm.NewChatClient(), embeddingMemo: map[string][]float64{}}

	first, ok := app.cachedQueryEmbedding(context.Background(), model.ModelConfig{EmbeddingBaseURL: firstServer.URL, EmbeddingModel: "same-model"}, "创建日历")
	if !ok || len(first) != 2 || first[0] != 1 {
		t.Fatalf("first embedding = %#v, %v", first, ok)
	}
	second, ok := app.cachedQueryEmbedding(context.Background(), model.ModelConfig{EmbeddingBaseURL: secondServer.URL, EmbeddingModel: "same-model"}, "创建日历")
	if !ok || len(second) != 2 || second[1] != 1 {
		t.Fatalf("second embedding = %#v, %v", second, ok)
	}
	if firstRequests.Load() != 1 || secondRequests.Load() != 1 {
		t.Fatalf("embedding requests = (%d, %d)", firstRequests.Load(), secondRequests.Load())
	}
}

func TestQueryEmbeddingCacheKeepsExactCapacity(t *testing.T) {
	server, requests := newEmbeddingTestServer(t, []float64{1, 2, 3})
	app := &Server{client: llm.NewChatClient(), embeddingMemo: map[string][]float64{}}
	for i := 0; i < maxQueryEmbeddingCacheEntries; i++ {
		app.embeddingMemo["seed-"+strconv.Itoa(i)] = []float64{1}
	}
	cfg := model.ModelConfig{EmbeddingBaseURL: server.URL, EmbeddingModel: "demo"}
	if _, ok := app.cachedQueryEmbedding(context.Background(), cfg, "new query"); !ok {
		t.Fatal("new query embedding was not cached")
	}
	if len(app.embeddingMemo) != maxQueryEmbeddingCacheEntries {
		t.Fatalf("embedding cache size = %d, want %d", len(app.embeddingMemo), maxQueryEmbeddingCacheEntries)
	}
	if _, ok := app.embeddingMemo[queryEmbeddingCacheKey(cfg, "new query")]; !ok {
		t.Fatal("new query cache entry missing")
	}
	if requests.Load() != 1 {
		t.Fatalf("embedding requests = %d", requests.Load())
	}
}
