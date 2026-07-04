package chatdock

import (
	"bytes"
	"chatdock/internal/chatdock/model"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuiltinScheduledTaskToolCRUD(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.callBuiltinScheduledTaskTool(context.Background(), builtinToolCreateScheduledTask, map[string]any{
		"title":            "日报",
		"prompt":           "总结今天",
		"schedule_type":    "interval",
		"interval_minutes": float64(15),
	})
	if err != nil {
		t.Fatal(err)
	}
	createdTasks := created.(model.ScheduledTaskResponse).Tasks
	if len(createdTasks) != 1 || createdTasks[0].ID == "" || !createdTasks[0].Enabled {
		t.Fatalf("unexpected created tasks: %#v", createdTasks)
	}
	id := createdTasks[0].ID

	listed, err := app.callBuiltinScheduledTaskTool(context.Background(), builtinToolListScheduledTasks, map[string]any{"query": "日报"})
	if err != nil {
		t.Fatal(err)
	}
	if tasks := listed.(model.ScheduledTaskResponse).Tasks; len(tasks) != 1 || tasks[0].ID != id {
		t.Fatalf("list should find created task: %#v", tasks)
	}

	updated, err := app.callBuiltinScheduledTaskTool(context.Background(), builtinToolUpdateScheduledTask, map[string]any{"id": id, "enabled": false, "time_of_day": "09:30", "schedule_type": "daily"})
	if err != nil {
		t.Fatal(err)
	}
	updatedTasks := updated.(model.ScheduledTaskResponse).Tasks
	if len(updatedTasks) != 1 || updatedTasks[0].Enabled || updatedTasks[0].ScheduleType != "daily" || updatedTasks[0].TimeOfDay != "09:30" {
		t.Fatalf("unexpected updated tasks: %#v", updatedTasks)
	}

	enabled, err := app.callBuiltinScheduledTaskTool(context.Background(), builtinToolUpdateScheduledTask, map[string]any{"id": id, "enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	if tasks := enabled.(model.ScheduledTaskResponse).Tasks; len(tasks) != 1 || !tasks[0].Enabled {
		t.Fatalf("update enabled should turn task on: %#v", tasks)
	}

	deleted, err := app.callBuiltinScheduledTaskTool(context.Background(), builtinToolDeleteScheduledTask, map[string]any{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	if tasks := deleted.(model.ScheduledTaskResponse).Tasks; len(tasks) != 0 {
		t.Fatalf("delete should remove task: %#v", tasks)
	}
}

func TestBuiltinScheduledTaskToolsExposeOnlyCRUD(t *testing.T) {
	tools := builtinScheduledTaskTools()
	names := map[string]bool{}
	updateHasEnabled := false
	for _, tool := range tools {
		names[tool.FullName] = true
		if tool.FullName != builtinToolUpdateScheduledTask {
			continue
		}
		props, _ := tool.InputSchema["properties"].(map[string]any)
		_, updateHasEnabled = props["enabled"]
	}
	if names["chatdock_scheduled_task_run"] || names["chatdock_scheduled_task_set_enabled"] {
		t.Fatalf("run/set_enabled should not be model-callable tools: %#v", names)
	}
	if !updateHasEnabled {
		t.Fatal("update tool should include enabled for enable/disable")
	}
}

func TestChatCanUseBuiltinScheduledTaskToolWithoutMCP(t *testing.T) {
	requestCount := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			tools, _ := body["tools"].([]any)
			found := false
			for _, raw := range tools {
				tool, _ := raw.(map[string]any)
				fn, _ := tool["function"].(map[string]any)
				if fn["name"] == builtinToolCreateScheduledTask {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("first model request should include builtin scheduled tool: %#v", body["tools"])
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"chatdock_scheduled_task_create","arguments":"{\"title\":\"日报\",\"prompt\":\"总结今天\",\"schedule_type\":\"interval\",\"interval_minutes\":30}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"已创建日报定时任务。"}}]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveModelConfig(model.ModelConfig{BaseURL: modelServer.URL, Model: "demo", SystemPrompt: "测试助手"}); err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{}`)))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create session status %d: %s", w.Code, w.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"session_id":"`+session.ID+`","message":"帮我每天总结"}`))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("chat status %d: %s", w.Code, w.Body.String())
	}
	tasks, err := app.store.ListScheduledTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].Title != "日报" || tasks.Tasks[0].IntervalMinutes != 30 {
		t.Fatalf("builtin tool should create scheduled task: %#v", tasks.Tasks)
	}
}
