package chatdock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"chatdock/internal/chatdock/model"
)

type sessionPageResponse struct {
	Sessions   []model.SessionSummary `json:"sessions"`
	NextCursor string                 `json:"next_cursor"`
	HasMore    bool                   `json:"has_more"`
}

func TestSessionListAPIUsesCursorPaginationWhenRequested(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		session, err := app.store.CreateSession("default")
		if err != nil {
			t.Fatal(err)
		}
		appendUserMessageForAppTest(t, app, "default", session.ID, "分页会话")
	}

	routes := app.routes()
	firstRecorder := httptest.NewRecorder()
	routes.ServeHTTP(firstRecorder, httptest.NewRequest(http.MethodGet, "/api/sessions?limit=2", nil))
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first page status %d: %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	var first sessionPageResponse
	if err := json.Unmarshal(firstRecorder.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Sessions) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %#v", first)
	}

	secondRecorder := httptest.NewRecorder()
	path := "/api/sessions?limit=2&cursor=" + url.QueryEscape(first.NextCursor)
	routes.ServeHTTP(secondRecorder, httptest.NewRequest(http.MethodGet, path, nil))
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("second page status %d: %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	var second sessionPageResponse
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Sessions) != 1 || second.HasMore || second.NextCursor != "" {
		t.Fatalf("unexpected second page: %#v", second)
	}
	seen := map[string]bool{}
	for _, item := range append(first.Sessions, second.Sessions...) {
		if seen[item.ID] {
			t.Fatalf("duplicate session %s across pages", item.ID)
		}
		seen[item.ID] = true
	}
}

func TestSessionListAPIRejectsInvalidPagination(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()
	for _, path := range []string{
		"/api/sessions?limit=0",
		"/api/sessions?limit=2&cursor=invalid",
		"/api/sessions/search?q=test&limit=2&cursor=invalid",
	} {
		recorder := httptest.NewRecorder()
		routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}
