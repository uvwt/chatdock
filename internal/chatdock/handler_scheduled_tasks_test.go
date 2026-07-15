package chatdock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"chatdock/internal/chatdock/model"
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

func TestScheduledTaskRescheduleSemantics(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := app.store.CreateScheduledTask("default", model.ScheduledTaskRequest{Title: "间隔任务", Prompt: "总结今天", Enabled: true, ScheduleType: "interval", IntervalMinutes: 60, ContextMode: model.ScheduledTaskContextStateless})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 {
		t.Fatalf("unexpected created tasks: %#v", created.Tasks)
	}
	task := created.Tasks[0]
	originalNextRun := task.NextRunAt
	if originalNextRun.IsZero() {
		t.Fatalf("created task should have next_run_at: %#v", task)
	}

	updated, err := app.store.UpdateScheduledTask("default", task.ID, model.ScheduledTaskRequest{Title: "间隔任务重命名", Prompt: "改一下内容", Enabled: true, ScheduleType: "interval", IntervalMinutes: 60, ContextMode: model.ScheduledTaskContextSession})
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.Tasks[0].NextRunAt; !got.Equal(originalNextRun) {
		t.Fatalf("content-only save should preserve next_run_at, got %s want %s", got, originalNextRun)
	}

	changedPlan, err := app.store.UpdateScheduledTask("default", task.ID, model.ScheduledTaskRequest{Title: "间隔任务重命名", Prompt: "改一下内容", Enabled: true, ScheduleType: "interval", IntervalMinutes: 90, ContextMode: model.ScheduledTaskContextSession})
	if err != nil {
		t.Fatal(err)
	}
	if got := changedPlan.Tasks[0].NextRunAt; got.Equal(originalNextRun) {
		t.Fatalf("schedule change should recalculate next_run_at, still %s", got)
	}
	planNextRun := changedPlan.Tasks[0].NextRunAt

	rescheduled, err := app.store.UpdateScheduledTask("default", task.ID, model.ScheduledTaskRequest{Title: "间隔任务重命名", Prompt: "改一下内容", Enabled: true, ScheduleType: "interval", IntervalMinutes: 90, ContextMode: model.ScheduledTaskContextSession, Reschedule: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := rescheduled.Tasks[0].NextRunAt; got.Equal(planNextRun) {
		t.Fatalf("explicit reschedule should recalculate next_run_at, still %s", got)
	}
}

func TestOnceScheduledTaskRescheduleRequiresRunAt(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.store.CreateScheduledTask("default", model.ScheduledTaskRequest{Title: "一次任务", Prompt: "执行一次", Enabled: true, ScheduleType: "once", RunAt: "2099-01-01T10:00"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.store.UpdateScheduledTask("default", created.Tasks[0].ID, model.ScheduledTaskRequest{Title: "一次任务", Prompt: "执行一次", Enabled: true, ScheduleType: "once", Reschedule: true})
	if err == nil {
		t.Fatal("once reschedule without a legal run_at should fail")
	}
}

func TestScheduledTaskRunWritesConversationDetails(t *testing.T) {
	var requestCount atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requestCount.Add(1)
		if requestNumber == 3 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":""}}]}`)
			return
		}
		if requestNumber == 4 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"定时任务执行结果"}}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer should support flushing")
		}
		send := func(payload string) {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
		switch requestNumber {
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
	if _, err := app.store.SaveModelConfig("default", model.ModelConfig{BaseURL: modelServer.URL, Model: "demo", SystemPrompt: "测试助手"}); err != nil {
		t.Fatal(err)
	}
	created, err := app.store.CreateScheduledTask("default", model.ScheduledTaskRequest{Title: "细节任务", Prompt: "请查工具再总结", Enabled: true, ScheduleType: "interval", IntervalMinutes: 15, ContextMode: model.ScheduledTaskContextStateless})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tasks) != 1 {
		t.Fatalf("unexpected created tasks: %#v", created.Tasks)
	}

	result, err := app.executeScheduledTask(context.Background(), "default", created.Tasks[0].ID, true)
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
	if result.Session.Title != "定时任务执行结果" {
		t.Fatalf("scheduled run should use AI generated session title, got %q", result.Session.Title)
	}
	if got := requestCount.Load(); got != 4 {
		t.Fatalf("scheduled title should retry once after empty model content, requests=%d", got)
	}
	summaries, err := app.store.ListSessions("default")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Fatalf("scheduled session should not appear in normal session list: %#v", summaries)
	}
	searchResults, err := app.store.SearchSessions("default", "定时任务", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(searchResults) != 0 {
		t.Fatalf("scheduled session should not appear in normal session search: %#v", searchResults)
	}
}
