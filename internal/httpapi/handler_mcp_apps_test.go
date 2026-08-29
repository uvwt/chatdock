package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"chatdock/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func newMCPAppBridgeTestServer(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	sdkServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "app-bridge-test", Version: "1"}, nil)
	add := func(name string, meta mcpsdk.Meta) {
		sdkServer.AddTool(&mcpsdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}, Meta: meta}, func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			*calls = append(*calls, name)
			return &mcpsdk.CallToolResult{
				Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}},
				StructuredContent: map[string]any{"tool": name},
			}, nil
		})
	}
	for _, name := range []string{"source", "safe", "confirm"} {
		add(name, nil)
	}
	add("app_only", mcpsdk.Meta{"ui": map[string]any{"visibility": []any{"app"}}})
	add("model_only", mcpsdk.Meta{"ui": map[string]any{"visibility": []any{"model"}}})
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return sdkServer }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(server.Close)
	return server
}

func postMCPAppCall(t *testing.T, app *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	if _, ok := body["session_id"]; !ok {
		session, err := app.store.CreateSession("")
		if err != nil {
			t.Fatal(err)
		}
		body["session_id"] = session.ID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/mcp/apps/call", bytes.NewReader(raw))
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	return response
}

func TestMCPAppToolCallStaysWithinSourceServerAndPreservesAuthorization(t *testing.T) {
	var calls []string
	upstream := newMCPAppBridgeTestServer(t, &calls)
	app := newResourceTestApp(t, mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"demo": {URL: upstream.URL, ConfirmTools: []string{"confirm"}},
	}})

	response := postMCPAppCall(t, app, map[string]any{
		"source_tool": "demo__source",
		"name":        "safe",
		"arguments":   map[string]any{"value": 1},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("safe app call status=%d body=%s", response.Code, response.Body.String())
	}
	var result mcp.MCPToolResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	structured, _ := result.StructuredContent.(map[string]any)
	if structured["tool"] != "safe" || !slices.Contains(calls, "safe") {
		t.Fatalf("safe app call result=%#v calls=%#v", result, calls)
	}

	response = postMCPAppCall(t, app, map[string]any{
		"source_tool": "demo__source",
		"name":        "app_only",
		"arguments":   map[string]any{},
	})
	if response.Code != http.StatusOK || !slices.Contains(calls, "app_only") {
		t.Fatalf("app-only target status=%d body=%s calls=%#v", response.Code, response.Body.String(), calls)
	}

	response = postMCPAppCall(t, app, map[string]any{
		"source_tool": "demo__source",
		"name":        "model_only",
		"arguments":   map[string]any{},
	})
	if response.Code != http.StatusForbidden || slices.Contains(calls, "model_only") {
		t.Fatalf("model-only target status=%d body=%s calls=%#v", response.Code, response.Body.String(), calls)
	}

	response = postMCPAppCall(t, app, map[string]any{
		"source_tool": "demo__source",
		"name":        "confirm",
		"arguments":   map[string]any{},
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("confirm-required app call status=%d body=%s", response.Code, response.Body.String())
	}
	if slices.Contains(calls, "confirm") {
		t.Fatalf("MCP App bypassed confirm policy: %#v", calls)
	}

	response = postMCPAppCall(t, app, map[string]any{
		"source_tool": "demo__source",
		"name":        "other__danger",
		"arguments":   map[string]any{},
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-server-shaped app call status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMCPAppToolCallIsBoundToSessionAndAudited(t *testing.T) {
	var calls []string
	upstream := newMCPAppBridgeTestServer(t, &calls)
	app := newResourceTestApp(t, mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"demo": {URL: upstream.URL},
	}})
	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}

	response := postMCPAppCall(t, app, map[string]any{
		"session_id":  session.ID,
		"source_tool": "demo__source",
		"name":        "safe",
		"arguments":   map[string]any{"value": 1},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("app call status=%d body=%s", response.Code, response.Body.String())
	}
	runs, err := app.store.ListMCPRuns(session.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 || runs.Runs[0].Status != "success" || runs.Runs[0].EventCount != 2 {
		t.Fatalf("audited runs = %#v", runs.Runs)
	}
	detail, err := app.store.MCPRunDetail(runs.Runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != 2 || detail.Events[0].Kind != "tool_call" || detail.Events[1].Kind != "tool_result" || detail.Events[1].Server != "demo" || detail.Events[1].Tool != "safe" {
		t.Fatalf("audited events = %#v", detail.Events)
	}

	response = postMCPAppCall(t, app, map[string]any{
		"session_id":  "missing-session",
		"source_tool": "demo__source",
		"name":        "safe",
		"arguments":   map[string]any{},
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing session status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMCPAppToolCallRejectsForgedSourceTool(t *testing.T) {
	var calls []string
	upstream := newMCPAppBridgeTestServer(t, &calls)
	app := newResourceTestApp(t, mcp.MCPConfig{Servers: map[string]mcp.MCPServerConfig{
		"demo": {URL: upstream.URL},
	}})
	response := postMCPAppCall(t, app, map[string]any{
		"source_tool": "demo__not_exposed",
		"name":        "safe",
		"arguments":   map[string]any{},
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("forged source status=%d body=%s", response.Code, response.Body.String())
	}
	if len(calls) != 0 {
		t.Fatalf("forged source caused upstream tool call: %#v", calls)
	}
}
