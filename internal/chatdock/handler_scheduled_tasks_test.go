package chatdock

import (
	"bytes"
	"chatdock/internal/chatdock/model"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestScheduledTaskRunWritesConversationDetails(t *testing.T) {
	var requestCount atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer should support flushing")
		}
		send := func(payload string) {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
		switch requestCount.Add(1) {
		case 1:
			send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"` + builtinToolSearchTools + `","arguments":"{\"query\":\"定时任务\"}"}}]}}]}`)
		case 2:
			send(`{"choices":[{"delta":{"reasoning_content":"先查工具。"}}]}`)
			send(`{"choices":[{"delta":{"content":"完成定时任务。"}}]}`)
		default:
			http.Error(w, "unexpected model request", http.StatusBadRequest)
			return
		}
		send(`[DONE]`)
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveModelConfig(model.ModelConfig{BaseURL: modelServer.URL, Model: "demo", SystemPrompt: "测试助手"}); err != nil {
		t.Fatal(err)
	}
	created, err := app.store.CreateScheduledTask(model.ScheduledTaskRequest{Title: "细节任务", Prompt: "请查工具再总结", Enabled: true, ScheduleType: "interval", IntervalMinutes: 15, ContextMode: model.ScheduledTaskContextStateless})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 {
		t.Fatalf("unexpected created tasks: %#v", created.Tasks)
	}

	result, err := app.executeScheduledTask(context.Background(), app.store.ActivePrompt(), created.Tasks[0].ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Session == nil || len(result.Session.Messages) != 2 {
		t.Fatalf("scheduled run should be a normal two-message session, got %#v", result.Session)
	}
	assistant := result.Session.Messages[1]
	if assistant.Role != "assistant" || assistant.Content != "完成定时任务。" {
		t.Fatalf("unexpected assistant message: %#v", assistant)
	}
	if assistant.Reasoning != "先查工具。" {
		t.Fatalf("scheduled run should persist reasoning, got %#v", assistant.Reasoning)
	}
	if len(assistant.Events) == 0 || assistant.Events[0].Kind != "tool" || assistant.Events[0].Phase != "done" {
		t.Fatalf("scheduled run should persist tool events, got %#v", assistant.Events)
	}
	if len(assistant.Parts) == 0 {
		t.Fatalf("scheduled run should persist message parts")
	}
}
