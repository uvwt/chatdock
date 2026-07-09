package chatdock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

func TestRuntimeContextProbeMessagesContainCapabilityContext(t *testing.T) {
	capabilityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"context":"# AgentDock Context\n- exec_command\n- skill_read\n- skill_run\n- workflow_template_manage\n- task_manage\n任务模板：workflow_template_manage match"}`))
	}))
	defer capabilityServer.Close()

	app := &App{cfg: model.ServerConfig{AgentDockContextURL: capabilityServer.URL}}
	history := app.appendAgentDockRuntimeContext(context.Background(), []model.Message{
		{Role: "user", Content: "older"},
		{Role: "assistant", Content: "older answer"},
		{Role: "user", Content: "latest"},
	})
	messages := llm.BuildChatMessagesAny(model.ModelConfig{ContextMode: model.ContextModeCustom, MaxContextMessages: 1}, history)
	if len(messages) != 2 {
		t.Fatalf("expected merged runtime system and latest user, got %#v", messages)
	}
	if messages[0]["role"] != "system" || !strings.Contains(toString(messages[0]["content"]), "AgentDock Context") {
		t.Fatalf("runtime system context should be first, got %#v", messages)
	}
	joined := ""
	for _, msg := range messages {
		joined += "\n" + strings.TrimSpace(toString(msg["content"]))
	}
	if !strings.Contains(joined, "exec_command") || !strings.Contains(joined, "workflow_template_manage") || !strings.Contains(joined, "task_manage") {
		t.Fatalf("capability details missing from model request: %#v", messages)
	}
	if messages[len(messages)-1]["role"] != "user" || messages[len(messages)-1]["content"] != "latest" {
		t.Fatalf("latest user message not preserved at end: %#v", messages)
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
