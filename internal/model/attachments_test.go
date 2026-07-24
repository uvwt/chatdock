package model

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildUserContentForModelDescribesAttachmentStates(t *testing.T) {
	attachments := []AttachmentRecord{
		{Attachment: Attachment{Name: "notes.txt", MIMEType: "text/plain", Size: 12}, TextContent: "  important context  "},
		{Attachment: Attachment{Name: "photo.PNG", MIMEType: "application/octet-stream", Size: 8}, ModelURL: "https://example.test/photo.png"},
		{Attachment: Attachment{Name: "empty.pdf", MIMEType: "application/pdf", Size: 0}},
	}
	content := BuildUserContentForModel("  请总结  ", attachments)
	for _, want := range []string{
		"附件 1：notes.txt",
		"important context",
		"附件 2：photo.PNG",
		"图片内容已随模型请求发送",
		"附件 3：empty.pdf",
		"未提取到文本内容",
		"## 用户问题\n请总结",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("attachment prompt missing %q:\n%s", want, content)
		}
	}

	fallback := BuildUserContentForModel("", attachments[:1])
	if !strings.HasSuffix(fallback, "请分析这些附件。") {
		t.Fatalf("empty question fallback missing: %s", fallback)
	}
	if got := BuildUserContentForModel("  plain question  ", nil); got != "plain question" {
		t.Fatalf("attachment-free content = %q", got)
	}
}

func TestImageContentBlocksUsesOnlyRenderableImages(t *testing.T) {
	blocks := ImageContentBlocks([]AttachmentRecord{
		{Attachment: Attachment{Name: "photo.png", MIMEType: "image/png"}, ModelURL: " https://example.test/photo.png "},
		{Attachment: Attachment{Name: "missing.jpg", MIMEType: "image/jpeg"}},
		{Attachment: Attachment{Name: "notes.txt", MIMEType: "text/plain"}, ModelURL: "https://example.test/notes.txt"},
	})
	if len(blocks) != 1 {
		t.Fatalf("image blocks = %#v", blocks)
	}
	imageURL, ok := blocks[0]["image_url"].(map[string]any)
	if !ok || imageURL["url"] != "https://example.test/photo.png" {
		t.Fatalf("unexpected image block: %#v", blocks[0])
	}
}

func TestAttachmentDataURLDetectsImageAndRejectsText(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "image.bin")
	imageBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	if err := os.WriteFile(imagePath, imageBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	dataURL, err := AttachmentDataURL(AttachmentRecord{Attachment: Attachment{MIMEType: "application/octet-stream"}, StoragePath: imagePath})
	if err != nil {
		t.Fatal(err)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	if dataURL != want {
		t.Fatalf("data URL = %q, want %q", dataURL, want)
	}

	textPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(textPath, []byte("plain text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AttachmentDataURL(AttachmentRecord{StoragePath: textPath}); err == nil || !strings.Contains(err.Error(), "not an image") {
		t.Fatalf("text attachment error = %v", err)
	}
	if _, err := AttachmentDataURL(AttachmentRecord{}); err == nil || !strings.Contains(err.Error(), "path is empty") {
		t.Fatalf("empty path error = %v", err)
	}
}

func TestLimitAttachmentContextPreservesUTF8Boundary(t *testing.T) {
	text := "a" + strings.Repeat("你", maxExtractedTextBytes/3+10)
	limited := LimitAttachmentContext(text)
	if !utf8.ValidString(limited) {
		t.Fatal("limited attachment context contains invalid UTF-8")
	}
	if !strings.Contains(limited, "文件内容过长，已截断") {
		t.Fatalf("truncation marker missing: suffix=%q", limited[len(limited)-80:])
	}
	if got := LimitAttachmentContext("  short text  "); got != "short text" {
		t.Fatalf("short context = %q", got)
	}
}
