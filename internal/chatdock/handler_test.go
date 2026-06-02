package chatdock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionRenameAndExportAPI(t *testing.T) {
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var s Session
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/sessions/"+s.ID+"/rename", bytes.NewReader([]byte(`{"title":"renamed"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("rename status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+s.ID+"/export?format=md", nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("export status %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", w.Header().Get("Content-Type"))
	}
}

func TestSkillsAPI(t *testing.T) {
	app, err := NewApp(ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader([]byte(`{"name":"写作","description":"中文写作","content":"使用短句。","enabled":true}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create skill status %d: %s", w.Code, w.Body.String())
	}
	var result SkillResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 || result.Skills[0].ID == "" {
		t.Fatalf("unexpected skill result: %#v", result)
	}

	id := result.Skills[0].ID
	r = httptest.NewRequest(http.MethodPut, "/api/skills/"+id, bytes.NewReader([]byte(`{"name":"写作","description":"中文写作","content":"使用短句。","enabled":false}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("update skill status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodDelete, "/api/skills/"+id, nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete skill status %d: %s", w.Code, w.Body.String())
	}
}
