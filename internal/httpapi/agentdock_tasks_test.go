package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"chatdock/internal/agentdock"
	"chatdock/internal/model"
)

func TestAgentDockTaskProxyUsesRuntimeAPIAndBearerToken(t *testing.T) {
	seen := make(chan *http.Request, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/runtime/tasks":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tasks": []map[string]any{{"id": "tsk_123", "title": "Build", "status": "active"}}, "count": 1})
		case "/internal/runtime/tasks/tsk_123":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "task": map[string]any{"id": "tsk_123", "steps": []map[string]any{{"id": "test", "status": "completed"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	app := newAgentTaskTestApp(t, model.ServerConfig{
		AgentDockContextURL:   upstream.URL + "/context",
		AgentDockContextToken: "agent-secret",
	})
	routes := app.routes()

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent-tasks?status=active&limit=12", nil)
	listResp := httptest.NewRecorder()
	routes.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var listPayload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listPayload); err != nil {
		t.Fatal(err)
	}
	if listPayload.Count != 1 {
		t.Fatalf("list count = %d", listPayload.Count)
	}
	listUpstream := <-seen
	if listUpstream.URL.Path != "/internal/runtime/tasks" || listUpstream.URL.Query().Get("status") != "active" || listUpstream.URL.Query().Get("limit") != "12" {
		t.Fatalf("unexpected list upstream request: %s", listUpstream.URL.String())
	}
	if got := listUpstream.Header.Get("Authorization"); got != "Bearer agent-secret" {
		t.Fatalf("authorization = %q", got)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent-tasks/tsk_123", nil)
	detailResp := httptest.NewRecorder()
	routes.ServeHTTP(detailResp, detailReq)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detailResp.Code, detailResp.Body.String())
	}
	detailUpstream := <-seen
	if detailUpstream.URL.Path != "/internal/runtime/tasks/tsk_123" {
		t.Fatalf("unexpected detail upstream path: %s", detailUpstream.URL.Path)
	}
}

func TestAgentDockTaskDeleteProxy(t *testing.T) {
	type deleteRequest struct {
		Method string
		Path   string
		Auth   string
	}
	seen := make(chan deleteRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- deleteRequest{Method: r.Method, Path: r.URL.Path, Auth: r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "action": "delete", "task_id": "tsk_123",
			"deleted_task": map[string]any{"id": "tsk_123", "title": "Delete me"},
		})
	}))
	defer upstream.Close()

	app := newAgentTaskTestApp(t, model.ServerConfig{
		AgentDockContextURL:   upstream.URL + "/context",
		AgentDockContextToken: "agent-secret",
	})
	resp := httptest.NewRecorder()
	app.routes().ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/agent-tasks/tsk_123", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", resp.Code, resp.Body.String())
	}
	request := <-seen
	if request.Method != http.MethodDelete || request.Path != "/internal/runtime/tasks/tsk_123" {
		t.Fatalf("unexpected upstream delete request: %#v", request)
	}
	if request.Auth != "Bearer agent-secret" {
		t.Fatalf("unexpected upstream authorization: %#v", request)
	}
}

func TestAgentDockTaskProxyValidationAndConfigurationErrors(t *testing.T) {
	app := newAgentTaskTestApp(t, model.ServerConfig{})
	routes := app.routes()

	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/api/agent-tasks?status=running", want: http.StatusBadRequest},
		{path: "/api/agent-tasks?limit=201", want: http.StatusBadRequest},
		{path: "/api/agent-tasks", want: http.StatusServiceUnavailable},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		resp := httptest.NewRecorder()
		routes.ServeHTTP(resp, req)
		if resp.Code != test.want {
			t.Fatalf("%s status = %d, want %d body=%s", test.path, resp.Code, test.want, resp.Body.String())
		}
	}
}

func TestSessionAgentTaskReturnsLatestAssociatedTask(t *testing.T) {
	seenPath := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"task": map[string]any{"id": "tsk_current", "title": "Current task", "status": "active"},
		})
	}))
	defer upstream.Close()

	app := newAgentTaskTestApp(t, model.ServerConfig{AgentDockContextURL: upstream.URL + "/context"})
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.store.AppendAssistantMessageWithParts(session.ID, "working", "", nil, []model.MessageEvent{
		taskManageProxyEvent("create", "tsk_old"),
		taskManageProxyEvent("checkpoint", "tsk_current"),
		taskManageProxyEvent("get", "tsk_read_only"),
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID+"/agent-task", nil)
	resp := httptest.NewRecorder()
	app.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("session task status = %d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Task.ID != "tsk_current" || seenPath != "/internal/runtime/tasks/tsk_current" {
		t.Fatalf("unexpected session task id=%q upstream=%q", payload.Task.ID, seenPath)
	}
}

func TestSessionAgentTaskReturnsNullWhenAssociatedTaskWasDeleted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "code": "TASK_NOT_FOUND", "error": "task not found"})
	}))
	defer upstream.Close()

	app := newAgentTaskTestApp(t, model.ServerConfig{AgentDockContextURL: upstream.URL + "/context"})
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.store.AppendAssistantMessageWithParts(session.ID, "working", "", nil, []model.MessageEvent{
		taskManageProxyEvent("create", "tsk_deleted"),
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := httptest.NewRecorder()
	app.routes().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID+"/agent-task", nil))
	if resp.Code != http.StatusOK || resp.Body.String() != "{\"ok\":true,\"task\":null}\n" {
		t.Fatalf("unexpected deleted session task response status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSessionAgentTaskReturnsNullWithoutAssociatedTask(t *testing.T) {
	app := newAgentTaskTestApp(t, model.ServerConfig{})
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID+"/agent-task", nil)
	resp := httptest.NewRecorder()
	app.routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || resp.Body.String() != "{\"ok\":true,\"task\":null}\n" {
		t.Fatalf("unexpected empty session task response status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSessionTaskIDFromDirectTaskManageEvent(t *testing.T) {
	event := model.MessageEvent{Details: map[string]any{
		"tool":      "DockMini.task_manage",
		"arguments": map[string]any{"action": "checkpoint", "task_id": "tsk_direct"},
		"result":    map[string]any{"task_id": "tsk_direct"},
	}}
	if got := agentdock.SessionTaskIDFromEvent(event); got != "tsk_direct" {
		t.Fatalf("direct task id = %q", got)
	}
	event.Details["arguments"] = map[string]any{"action": "get", "task_id": "tsk_read_only"}
	if got := agentdock.SessionTaskIDFromEvent(event); got != "" {
		t.Fatalf("read-only task lookup must not bind session, got %q", got)
	}
}

func taskManageProxyEvent(action, taskID string) model.MessageEvent {
	arguments := map[string]any{"action": action}
	if action != "create" {
		arguments["task_id"] = taskID
	}
	outerArguments := map[string]any{"name": "DockMini__task_manage", "arguments": arguments}
	outerResult := map[string]any{
		"tool":   "DockMini__task_manage",
		"result": map[string]any{"task_id": taskID},
	}
	return model.MessageEvent{
		Kind:  "tool",
		Phase: "done",
		Text:  "调用完成：chatdock_tool_execute",
		Meta:  "DockMini__task_manage",
		Details: map[string]any{
			"event":     "tool_call_result",
			"tool":      "chatdock_tool_execute",
			"ok":        true,
			"arguments": outerArguments,
			"result":    outerResult,
			"data": map[string]any{
				"tool":      "chatdock_tool_execute",
				"arguments": outerArguments,
				"result":    outerResult,
			},
		},
	}
}

func newAgentTaskTestApp(t *testing.T, cfg model.ServerConfig) *Server {
	t.Helper()
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>ChatDock</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Addr = "127.0.0.1:0"
	cfg.DataDir = t.TempDir()
	cfg.WebDir = webDir
	app, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return app
}
