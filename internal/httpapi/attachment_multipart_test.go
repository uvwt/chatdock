package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"chatdock/internal/model"
)

type repeatingByteReader byte

func (r repeatingByteReader) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = byte(r)
	}
	return len(buffer), nil
}

func TestUploadSpillsLargeMultipartFileToDiskAndRemovesTemporaryFile(t *testing.T) {
	multipartTempDir := t.TempDir()
	t.Setenv("TMPDIR", multipartTempDir)
	app, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "large.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(part, bytes.NewReader(bytes.Repeat([]byte{'x'}, maxMultipartMemory+1)), maxMultipartMemory+1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/files", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("upload status %d: %s", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(multipartTempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("multipart temporary files remain after request completion: %#v", entries)
	}
}

func TestUploadReturnsPayloadTooLargeForRequestsOverLimit(t *testing.T) {
	app, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	boundary := "chatdock-over-limit"
	prefix := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"large.bin\"\r\nContent-Type: application/octet-stream\r\n\r\n", boundary)
	suffix := fmt.Sprintf("\r\n--%s--\r\n", boundary)
	body := io.MultiReader(strings.NewReader(prefix), io.LimitReader(repeatingByteReader('x'), maxUploadBytes), strings.NewReader(suffix))
	request := httptest.NewRequest(http.MethodPost, "/api/files", body)
	request.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	response := httptest.NewRecorder()

	app.routes().ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload status %d, want %d: %s", response.Code, http.StatusRequestEntityTooLarge, response.Body.String())
	}
}
