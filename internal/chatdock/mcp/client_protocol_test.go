package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseMCPConfigRejectsAmbiguousServerToolAliases(t *testing.T) {
	for _, content := range []string{
		`{"servers":{"":{"url":"https://example.test"}}}`,
		`{"servers":{"calendar prod":{},"calendar_prod":{}}}`,
	} {
		if _, err := ParseMCPConfig(content); err == nil {
			t.Fatalf("expected invalid server name aliases to fail: %s", content)
		}
	}
}

func TestCallToolResolvesSanitizedServerAliasToOriginalConfigName(t *testing.T) {
	var calledTool string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Method != "tools/call" {
			t.Errorf("method = %q", request.Method)
		}
		calledTool = request.Params.Name
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"ok": true}})
	}))
	defer server.Close()

	cfg := MCPConfig{Servers: map[string]MCPServerConfig{
		"calendar prod": {URL: server.URL},
	}}
	fullName := ToolFullName("calendar prod", "events_create")
	result, err := NewMCPClient().CallTool(context.Background(), cfg, fullName, map[string]any{"title": "周会"})
	if err != nil {
		t.Fatal(err)
	}
	if calledTool != "events_create" {
		t.Fatalf("called tool = %q", calledTool)
	}
	if !strings.Contains(CompactJSON(result), `"ok":true`) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestReadMCPResponseBodyRejectsDeclaredAndStreamingOverflow(t *testing.T) {
	declared := &http.Response{ContentLength: 33, Body: io.NopCloser(strings.NewReader("small"))}
	if _, err := readMCPResponseBody(declared, 32); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("declared overflow error = %v", err)
	}
	streamed := &http.Response{ContentLength: -1, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 33)))}
	if _, err := readMCPResponseBody(streamed, 32); err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
		t.Fatalf("streaming overflow error = %v", err)
	}
}

func TestMCPHTTPErrorBodyIsWhitespaceNormalizedAndBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(" upstream\n\t" + strings.Repeat("错误", maxMCPErrorSummaryRunes+100)))
	}))
	defer server.Close()
	cfg := MCPConfig{Servers: map[string]MCPServerConfig{"demo": {URL: server.URL}}}

	_, err := NewMCPClient().ListServerTools(context.Background(), cfg, "demo")
	if err == nil {
		t.Fatal("expected upstream HTTP error")
	}
	message := err.Error()
	if !strings.Contains(message, "502 Bad Gateway: upstream ") || !strings.HasSuffix(message, "…") {
		t.Fatalf("unexpected summarized error: %q", message)
	}
	if utf8.RuneCountInString(message) > maxMCPErrorSummaryRunes+100 {
		t.Fatalf("MCP error body was not bounded: %d runes", utf8.RuneCountInString(message))
	}
}
