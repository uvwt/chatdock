package chatdock

import (
	"bytes"
	"chatdock/internal/chatdock/model"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUploadAttachmentInjectsTextIntoModelContext(t *testing.T) {
	seen := make(chan string, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for i := len(body.Messages) - 1; i >= 0; i-- {
			if body.Messages[i]["role"] == "user" {
				seen <- body.Messages[i]["content"]
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"已读取附件"}}]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveModelConfig("default", model.ModelConfig{BaseURL: modelServer.URL, Model: "demo", SystemPrompt: "测试助手"}); err != nil {
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

	var uploadBody bytes.Buffer
	mw := multipart.NewWriter(&uploadBody)
	_ = mw.WriteField("session_id", session.ID)
	part, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "附件里的关键内容：青山计划")
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/files", &uploadBody)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("upload status %d: %s", w.Code, w.Body.String())
	}
	var uploaded model.FileUploadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if !uploaded.Attachment.HasText || uploaded.Attachment.Name != "note.txt" {
		t.Fatalf("unexpected uploaded attachment: %#v", uploaded.Attachment)
	}

	payload := `{"session_id":"` + session.ID + `","message":"请总结附件","attachment_ids":["` + uploaded.Attachment.ID + `"]}`
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(payload))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("chat status %d: %s", w.Code, w.Body.String())
	}
	got := <-seen
	if !strings.Contains(got, "附件里的关键内容：青山计划") || !strings.Contains(got, "请总结附件") {
		t.Fatalf("model context did not include attachment and question: %s", got)
	}
	var result model.ChatResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Session.Messages) == 0 || len(result.Session.Messages[0].Attachments) != 1 {
		t.Fatalf("visible session did not keep attachment card: %#v", result.Session.Messages)
	}
	if strings.Contains(result.Session.Messages[0].Content, "青山计划") {
		t.Fatalf("visible message should not be replaced by injected file content: %s", result.Session.Messages[0].Content)
	}
}

func TestUploadImageAttachmentSendsMultimodalContent(t *testing.T) {
	t.Skip("covered by llm BuildChatMessagesAny image message structure test")
	seen := make(chan map[string]any, 1)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for i := len(body.Messages) - 1; i >= 0; i-- {
			if body.Messages[i]["role"] == "user" {
				seen <- body.Messages[i]
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"看到了图片"}}]}`))
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web", PublicBaseURL: "https://chatdock.200399.xyz"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.SaveModelConfig("default", model.ModelConfig{BaseURL: modelServer.URL, Model: "demo", SystemPrompt: "测试助手"}); err != nil {
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

	var uploadBody bytes.Buffer
	mw := multipart.NewWriter(&uploadBody)
	_ = mw.WriteField("session_id", session.ID)
	part, err := mw.CreateFormFile("file", "pixel.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x0d})
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/files", &uploadBody)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("upload status %d: %s", w.Code, w.Body.String())
	}
	var uploaded model.FileUploadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}

	payload := `{"session_id":"` + session.ID + `","message":"这张图是什么","attachment_ids":["` + uploaded.Attachment.ID + `"]}`
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(payload))
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("chat status %d: %s", w.Code, w.Body.String())
	}
	msg := <-seen
	blocks, ok := msg["content"].([]any)
	if !ok || len(blocks) < 2 {
		t.Fatalf("expected multimodal content blocks, got %#v", msg["content"])
	}
	var hasText, hasImage bool
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		if block["type"] == "text" && strings.Contains(block["text"].(string), "这张图是什么") {
			hasText = true
		}
		if block["type"] == "image_url" {
			imageURL, _ := block["image_url"].(map[string]any)
			url, _ := imageURL["url"].(string)
			hasImage = strings.HasPrefix(url, "https://chatdock.200399.xyz/api/model-images/") && strings.Contains(url, "expires=") && strings.Contains(url, "sig=")
		}
	}
	if !hasText || !hasImage {
		t.Fatalf("expected text and image blocks, got %#v", blocks)
	}
}

func TestDownloadUploadedAttachment(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	routes := app.routes()

	var uploadBody bytes.Buffer
	mw := multipart.NewWriter(&uploadBody)
	part, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "可下载的附件内容")
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/files", &uploadBody)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("upload status %d: %s", w.Code, w.Body.String())
	}
	var uploaded model.FileUploadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/files/"+uploaded.Attachment.ID, nil)
	routes.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("download status %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "可下载的附件内容" {
		t.Fatalf("unexpected downloaded body: %q", got)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "note.txt") {
		t.Fatalf("unexpected content disposition: %q", cd)
	}
}

func TestModelImageURLIncludesFilenameExtension(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web", PublicBaseURL: "https://chatdock.200399.xyz"})
	if err != nil {
		t.Fatal(err)
	}
	url, err := app.modelImageURL("att_1", "pixel.png", time.Unix(1893456000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "https://chatdock.200399.xyz/api/model-images/att_1/pixel.png?") {
		t.Fatalf("expected image URL to include filename extension, got %s", url)
	}
	if !strings.Contains(url, "expires=") || !strings.Contains(url, "sig=") {
		t.Fatalf("expected signed URL, got %s", url)
	}
}
