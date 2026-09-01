package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatdock/internal/mcp"
	"chatdock/internal/model"
)

func TestBuildProviderSystemPromptMatchesRealRequestComposition(t *testing.T) {
	history := make([]model.Message, 0, 15)
	for i := 1; i <= 14; i++ {
		history = append(history, model.Message{Role: "user", Content: fmt.Sprintf("message-%02d %s", i, strings.Repeat("上下文 ", 500))})
	}
	history = append(history, model.Message{Role: "system", Content: "AgentDock Capability Context"})

	prompt := BuildProviderSystemPrompt(
		model.ModelConfig{SystemPrompt: "全局提示词\n\n项目提示词", ContextWindowTokens: 8 * 1024, OutputReserveTokens: 1024},
		history,
		[]mcp.MCPTool{{Name: "read", FullName: "agentdock__read"}},
	)

	for _, want := range []string{
		"全局提示词",
		"项目提示词",
		"# 早期会话摘要",
		"message-01",
		"AgentDock Capability Context",
		"ChatDock 工具资源已接入",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("provider system prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildProviderSystemPromptReturnsEmptyWithoutSystemMessagesOrTools(t *testing.T) {
	prompt := BuildProviderSystemPrompt(model.ModelConfig{}, []model.Message{{Role: "user", Content: "hello"}}, nil)
	if prompt != "" {
		t.Fatalf("provider system prompt = %q, want empty", prompt)
	}
}

func TestBuildProviderSystemPromptEqualsTheSystemMessageSentToProvider(t *testing.T) {
	var sentPrompt string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode provider request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		messages, _ := body["messages"].([]any)
		if len(messages) == 0 {
			t.Error("provider request has no messages")
			http.Error(w, "missing messages", http.StatusBadRequest)
			return
		}
		first, _ := messages[0].(map[string]any)
		sentPrompt, _ = first["content"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer provider.Close()

	cfg := model.ModelConfig{BaseURL: provider.URL, Model: "demo", SystemPrompt: "基础提示词"}
	history := []model.Message{
		{Role: "system", Content: "AgentDock Capability Context"},
		{Role: "user", Content: "hello"},
	}
	tools := []mcp.MCPTool{{Name: "read", FullName: "agentdock__read"}}
	expected := BuildProviderSystemPrompt(cfg, history, tools)

	client := NewChatClient()
	if _, err := client.CompleteWithMCPToolsEvents(context.Background(), cfg, history, tools, func(string, map[string]any) (any, error) {
		return nil, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	if sentPrompt != expected {
		t.Fatalf("sent system prompt differs from preview:\nsent: %q\npreview: %q", sentPrompt, expected)
	}
}

func TestMCPServerInstructionsReachProviderWithoutVisibleTools(t *testing.T) {
	var sentMessages []map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		sentMessages = body.Messages
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer provider.Close()

	cfg := model.ModelConfig{BaseURL: provider.URL, Model: "demo"}
	instructions := []mcp.MCPServerInstruction{{Server: "agentdock", Instructions: "Call agentdock_context before using other capabilities."}}
	client := NewChatClient()
	answer, err := client.CompleteWithMCPToolsEvents(
		context.Background(),
		cfg,
		[]model.Message{{Role: "user", Content: "hello"}},
		nil,
		nil,
		nil,
		MCPToolLoopOptions{ServerInstructions: instructions},
	)
	if err != nil || answer != "ok" {
		t.Fatalf("answer=%q err=%v", answer, err)
	}
	if len(sentMessages) < 2 || sentMessages[0]["role"] != "system" {
		t.Fatalf("provider messages=%#v", sentMessages)
	}
	prompt, _ := sentMessages[0]["content"].(string)
	if !strings.Contains(prompt, "Call agentdock_context") || !strings.Contains(prompt, "allow/deny/confirm") {
		t.Fatalf("provider did not receive scoped MCP instructions: %q", prompt)
	}
}
