package chatdock

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestRemovedSettingsRunAndAgentRoutes(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>ChatDock</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: webDir})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	for _, path := range []string{"/api/runs", "/api/runs/test", "/api/prompts", "/api/prompts/select"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status %d, want 404", path, w.Code)
		}
	}
}
