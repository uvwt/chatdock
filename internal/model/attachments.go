package model

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxExtractedTextBytes = 180 << 10

func BuildUserContentForModel(content string, attachments []AttachmentRecord) string {
	content = strings.TrimSpace(content)
	if len(attachments) == 0 {
		return content
	}
	var b strings.Builder
	b.WriteString("用户上传了以下附件。请优先结合附件内容回答；图片附件会作为视觉内容随模型请求发送。\n")
	for i, item := range attachments {
		fmt.Fprintf(&b, "\n## 附件 %d：%s\n", i+1, item.Name)
		fmt.Fprintf(&b, "- 类型：%s\n- 大小：%s\n", item.MIMEType, humanBytes(item.Size))
		text := strings.TrimSpace(item.TextContent)
		if text == "" {
			if IsImageAttachment(item) {
				b.WriteString("- 状态：图片内容已随模型请求发送。\n")
			} else {
				b.WriteString("- 状态：已上传，但未提取到文本内容。\n")
			}
			continue
		}
		b.WriteString("\n```text\n")
		b.WriteString(LimitAttachmentContext(text))
		b.WriteString("\n```\n")
	}
	b.WriteString("\n## 用户问题\n")
	if content == "" {
		content = "请分析这些附件。"
	}
	b.WriteString(content)
	return b.String()
}

func ImageContentBlocks(attachments []AttachmentRecord) []map[string]any {
	blocks := make([]map[string]any, 0, len(attachments))
	for _, item := range attachments {
		if !IsImageAttachment(item) {
			continue
		}
		url := strings.TrimSpace(item.ModelURL)
		if url == "" {
			continue
		}
		blocks = append(blocks, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": url,
			},
		})
	}
	return blocks
}
func IsImageAttachment(item AttachmentRecord) bool {
	mimeType := strings.ToLower(strings.TrimSpace(item.MIMEType))
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(item.Name))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".gif"
}

func AttachmentDataURL(item AttachmentRecord) (string, error) {
	path := strings.TrimSpace(item.StoragePath)
	if path == "" {
		return "", fmt.Errorf("attachment storage path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mimeType := strings.TrimSpace(item.MIMEType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(raw)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "", fmt.Errorf("attachment is not an image: %s", mimeType)
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

func LimitAttachmentContext(text string) string {
	raw := []byte(strings.TrimSpace(text))
	if len(raw) <= maxExtractedTextBytes {
		return string(raw)
	}
	cut := raw[:maxExtractedTextBytes]
	for !utf8.Valid(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return string(cut) + "\n\n[文件内容过长，已截断，仅注入前 " + humanBytes(maxExtractedTextBytes) + "]"
}

func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", n, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
