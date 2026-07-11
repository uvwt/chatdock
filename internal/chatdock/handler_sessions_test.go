package chatdock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestSessionRenameAndExportAPI(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
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
	var s model.Session
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

func TestSessionCloneAPI(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := app.store.AppendUserMessage("default", session.ID, "需要复制的消息"); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/sessions/"+session.ID+"/clone", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("clone status %d: %s", w.Code, w.Body.String())
	}
	var cloned model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &cloned); err != nil {
		t.Fatal(err)
	}
	if cloned.ID == session.ID || len(cloned.Messages) != 1 || !strings.Contains(cloned.Title, "副本") {
		t.Fatalf("unexpected cloned session: %#v", cloned)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", w.Code, w.Body.String())
	}
	var summaries []model.SessionSummary
	if err := json.Unmarshal(w.Body.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries[0].Preview == "" {
		t.Fatalf("expected cloned session and preview in list: %#v", summaries)
	}
}

func TestSessionBranchAPI(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := app.store.AppendUserMessage("default", session.ID, "需要分支的用户消息"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.AppendAssistantMessage("default", session.ID, "需要分支的助手回复"); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/sessions/"+session.ID+"/branch", bytes.NewReader([]byte(`{"message_index":0}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("branch status %d: %s", w.Code, w.Body.String())
	}
	var branched model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &branched); err != nil {
		t.Fatal(err)
	}
	if branched.ID == session.ID || len(branched.Messages) != 1 || !strings.Contains(branched.Title, "分支") {
		t.Fatalf("unexpected branched session: %#v", branched)
	}
}

func TestSessionDeleteAPI(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodDelete, "/api/sessions/"+session.ID, nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID, nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("deleted session should be 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionEditUserMessageTruncatesFollowingMessages(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := app.store.AppendUserMessage("default", session.ID, "原始问题"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.AppendAssistantMessage("default", session.ID, "后续回答"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := app.store.AppendUserMessage("default", session.ID, "后续追问"); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewReader([]byte(`{"message_index":0,"content":"改后的问题"}`))
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/sessions/"+session.ID+"/messages", body)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("edit status %d: %s", w.Code, w.Body.String())
	}
	var edited model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &edited); err != nil {
		t.Fatal(err)
	}
	if len(edited.Messages) != 1 {
		t.Fatalf("expected following messages truncated, got %d messages: %#v", len(edited.Messages), edited.Messages)
	}
	if edited.Messages[0].Role != "user" || edited.Messages[0].Content != "改后的问题" {
		t.Fatalf("unexpected edited message: %#v", edited.Messages[0])
	}
}

func TestSessionModelAPI(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/sessions/"+session.ID+"/model", bytes.NewReader([]byte(`{"provider_id":"provider-a","model":"model-x"}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("model status %d: %s", w.Code, w.Body.String())
	}
	var updated model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ProviderID != "provider-a" || updated.Model != "model-x" {
		t.Fatalf("unexpected updated session: %#v", updated)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID, nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("get status %d: %s", w.Code, w.Body.String())
	}
	var loaded model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.ProviderID != "provider-a" || loaded.Model != "model-x" {
		t.Fatalf("session model should survive reload: %#v", loaded)
	}
}

func TestSessionGetCompactsToolEventDetailsAndLazyLoadsFullEvent(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", 20000)
	_, err = app.store.AppendAssistantMessageWithParts("default", session.ID, "done", "", nil, []model.MessageEvent{{
		Kind: "tool", Phase: "done", Text: "调用完成：chatdock_tool_execute", Details: map[string]any{
			"event":     "tool_call_result",
			"tool":      "chatdock_tool_execute",
			"ok":        true,
			"arguments": map[string]any{"name": "DockMini.exec_command", "arguments": map[string]any{"cmd": huge}},
			"result":    map[string]any{"tool": "DockMini.exec_command", "result": huge},
			"data":      map[string]any{"tool": "chatdock_tool_execute", "arguments": map[string]any{"name": "DockMini.exec_command"}, "result": map[string]any{"tool": "DockMini.exec_command", "result": huge}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID, nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("get status %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), huge) {
		t.Fatalf("compact session response still contains huge detail")
	}
	var compact model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &compact); err != nil {
		t.Fatal(err)
	}
	if len(compact.Messages) != 1 || len(compact.Messages[0].Events) != 1 {
		t.Fatalf("unexpected compact messages: %#v", compact.Messages)
	}
	details := compact.Messages[0].Events[0].Details
	eventID, _ := details["event_id"].(string)
	if compact.Messages[0].Events[0].ID == "" || eventID == "" || compact.Messages[0].Events[0].ID != eventID || details["lazy"] != true || details["message_index"] == nil || details["event_index"] == nil {
		t.Fatalf("expected lazy event_id ref, got event=%#v details=%#v", compact.Messages[0].Events[0], details)
	}
	args, _ := details["arguments"].(map[string]any)
	if args["name"] != "DockMini.exec_command" {
		t.Fatalf("compact event should keep display name, got %#v", details)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID+"/tool-events/"+eventID, nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("lazy event id status %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), huge) {
		t.Fatalf("lazy detail response should contain full original detail")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/sessions/"+session.ID+"/tool-event?message_index=0&event_index=0", nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy lazy event status %d: %s", w.Code, w.Body.String())
	}
}
