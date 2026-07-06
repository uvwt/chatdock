package chatdock

import (
	"bytes"
	"chatdock/internal/chatdock/model"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScheduledTasksAPI(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/scheduled-tasks", bytes.NewReader([]byte(`{"title":"日报","prompt":"总结今天","enabled":true,"schedule_type":"interval","interval_minutes":15}`)))
	w := httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create scheduled task status %d: %s", w.Code, w.Body.String())
	}
	var result model.ScheduledTaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 || result.Tasks[0].ID == "" {
		t.Fatalf("unexpected scheduled task result: %#v", result)
	}

	id := result.Tasks[0].ID
	r = httptest.NewRequest(http.MethodPut, "/api/scheduled-tasks/"+id, bytes.NewReader([]byte(`{"title":"日报","prompt":"总结今天","enabled":false,"schedule_type":"daily","time_of_day":"09:30"}`)))
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("update scheduled task status %d: %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequest(http.MethodDelete, "/api/scheduled-tasks/"+id, nil)
	w = httptest.NewRecorder()
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete scheduled task status %d: %s", w.Code, w.Body.String())
	}
}
