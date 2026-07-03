package chatdock

import (
	"bytes"
	"chatdock/internal/chatdock/model"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelProviderTestUsesRequestConfig(t *testing.T) {
	seenPath := make(chan string, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seenPath <- r.URL.Path:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	body := `{"base_url":"` + modelServer.URL + `","model":"draft-model","api_key":""}`
	r := httptest.NewRequest(http.MethodPost, "/api/model-providers/test", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("model provider test status %d: %s", w.Code, w.Body.String())
	}
	if got := <-seenPath; got != "/models" {
		t.Fatalf("unexpected model test path: %s", got)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["model"] != "draft-model" {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func TestModelProviderTestKeepsSavedAPIKeyWhenMasked(t *testing.T) {
	seenAuth := make(chan string, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seenAuth <- r.Header.Get("Authorization"):
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveModelConfig(model.ModelConfig{BaseURL: "http://127.0.0.1:1/v1", Model: "saved", APIKey: "saved-secret"}); err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	body := `{"base_url":"` + modelServer.URL + `","model":"draft-model","api_key":"********"}`
	r := httptest.NewRequest(http.MethodPost, "/api/model-providers/test", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("model provider masked key test status %d: %s", w.Code, w.Body.String())
	}
	if got := <-seenAuth; got != "Bearer saved-secret" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func TestModelProviderModelsReturnsAvailableNames(t *testing.T) {
	seenPath := make(chan string, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seenPath <- r.URL.Path:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"zeta-model"},{"id":""},{"id":"alpha-model"},{"id":"zeta-model"}]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	body := `{"base_url":"` + modelServer.URL + `","model":"draft-model","api_key":""}`
	r := httptest.NewRequest(http.MethodPost, "/api/model-providers/models", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("model provider models status %d: %s", w.Code, w.Body.String())
	}
	if got := <-seenPath; got != "/models" {
		t.Fatalf("unexpected model list path: %s", got)
	}
	var result struct {
		OK     bool     `json:"ok"`
		Models []string `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	want := []string{"zeta-model", "alpha-model"}
	if !result.OK || len(result.Models) != len(want) {
		t.Fatalf("unexpected model list response: %#v", result)
	}
	for i := range want {
		if result.Models[i] != want[i] {
			t.Fatalf("unexpected model %d: got %q want %q; all=%#v", i, result.Models[i], want[i], result.Models)
		}
	}
}

func TestModelProviderModelsHidesHTMLChallengeBody(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={};</script>cloudflare challenge-platform very long body</body></html>`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	body := `{"base_url":"` + modelServer.URL + `","model":"draft-model","api_key":""}`
	r := httptest.NewRequest(http.MethodPost, "/api/model-providers/models", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("model provider html challenge status %d: %s", w.Code, w.Body.String())
	}
	response := w.Body.String()
	if strings.Contains(response, "<!DOCTYPE") || strings.Contains(response, "_cf_chl_opt") || strings.Contains(response, "challenge-platform") {
		t.Fatalf("response leaked html challenge body: %s", response)
	}
	if !strings.Contains(response, "Cloudflare") || !strings.Contains(response, "Base URL") {
		t.Fatalf("response should explain html challenge/base url problem: %s", response)
	}
}

func TestModelProviderModelsHidesNonJSONBody(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`not-json-response`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	body := `{"base_url":"` + modelServer.URL + `","model":"draft-model","api_key":""}`
	r := httptest.NewRequest(http.MethodPost, "/api/model-providers/models", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("model provider non-json status %d: %s", w.Code, w.Body.String())
	}
	response := w.Body.String()
	if !strings.Contains(response, "没有返回合法 JSON") || !strings.Contains(response, "not-json-response") {
		t.Fatalf("unexpected non-json response: %s", response)
	}
}

func TestModelProviderModelsKeepsSavedAPIKeyWhenMasked(t *testing.T) {
	seenAuth := make(chan string, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seenAuth <- r.Header.Get("Authorization"):
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"saved-key-model"}]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveModelConfig(model.ModelConfig{BaseURL: "http://127.0.0.1:1/v1", Model: "saved", APIKey: "saved-secret"}); err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	body := `{"base_url":"` + modelServer.URL + `","model":"draft-model","api_key":"********"}`
	r := httptest.NewRequest(http.MethodPost, "/api/model-providers/models", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("model provider masked key models status %d: %s", w.Code, w.Body.String())
	}
	if got := <-seenAuth; got != "Bearer saved-secret" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
	if strings.Contains(w.Body.String(), "saved-secret") {
		t.Fatalf("response leaked saved API key: %s", w.Body.String())
	}
}
