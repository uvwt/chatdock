package chatdock

import (
	"archive/zip"
	"bytes"
	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxUploadBytes        = 32 << 20
	maxExtractedTextBytes = 180 << 10
)

func (a *App) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("文件过大或表单不合法：%w", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("缺少 file 字段：%w", err))
		return
	}
	defer file.Close()

	name := cleanUploadName(header.Filename)
	id := model.NewID()
	sessionID := strings.TrimSpace(r.FormValue("session_id"))
	prompt := a.store.ActivePrompt()
	uploadDir := filepath.Join(a.cfg.DataDir, "uploads", safeFileComponent(prompt))
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	storagePath := filepath.Join(uploadDir, id+"_"+name)
	out, err := os.Create(storagePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	hash := sha256.New()
	written, copyErr := io.Copy(out, io.TeeReader(file, hash))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(storagePath)
		writeError(w, http.StatusInternalServerError, copyErr)
		return
	}
	if closeErr != nil {
		_ = os.Remove(storagePath)
		writeError(w, http.StatusInternalServerError, closeErr)
		return
	}
	if written <= 0 {
		_ = os.Remove(storagePath)
		writeError(w, http.StatusBadRequest, fmt.Errorf("文件为空"))
		return
	}

	mimeType := llm.FirstNonEmptyString(header.Header.Get("Content-Type"), mime.TypeByExtension(strings.ToLower(filepath.Ext(name))), "application/octet-stream")
	text, status, extractErr := extractAttachmentText(storagePath, name, mimeType)
	if extractErr != nil && strings.TrimSpace(text) == "" {
		status = "stored"
	}
	record := model.AttachmentRecord{
		Attachment: model.Attachment{
			ID:        id,
			Name:      name,
			MIMEType:  mimeType,
			Size:      written,
			Status:    status,
			HasText:   strings.TrimSpace(text) != "",
			TextBytes: len([]byte(text)),
			CreatedAt: time.Now(),
		},
		Prompt:      prompt,
		SessionID:   sessionID,
		StoragePath: storagePath,
		SHA256:      hex.EncodeToString(hash.Sum(nil)),
		TextContent: text,
	}
	if err := a.store.SaveAttachment(record); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, model.FileUploadResponse{Attachment: record.Attachment})
}

func cleanUploadName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "upload"
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '/' || r == '\\' || r == ':' {
			return '-'
		}
		return r
	}, name)
	if len([]rune(name)) > 120 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		runes := []rune(base)
		if len(runes) > 100 {
			base = string(runes[:100])
		}
		name = base + ext
	}
	return name
}

func safeFileComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "default"
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, value)
	value = strings.Trim(value, ".-")
	if value == "" {
		return "default"
	}
	return value
}

func extractAttachmentText(path string, name string, mimeType string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	lowerMime := strings.ToLower(strings.Split(mimeType, ";")[0])
	switch {
	case isPlainTextAttachment(ext, lowerMime):
		text, err := readTextFile(path)
		return text, "extracted", err
	case ext == ".docx" || lowerMime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		text, err := extractDocxText(path)
		return text, statusForText(text), err
	case ext == ".pdf" || lowerMime == "application/pdf":
		text, err := extractPDFTextBestEffort(path)
		return text, statusForText(text), err
	default:
		return "", "stored", nil
	}
}

func statusForText(text string) string {
	if strings.TrimSpace(text) == "" {
		return "stored"
	}
	return "extracted"
}

func isPlainTextAttachment(ext string, mimeType string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	plainExts := map[string]bool{
		".txt": true, ".md": true, ".markdown": true, ".json": true, ".jsonl": true, ".csv": true, ".tsv": true, ".log": true,
		".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".py": true, ".rb": true, ".php": true, ".java": true,
		".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cs": true, ".rs": true, ".swift": true, ".kt": true, ".sh": true,
		".yaml": true, ".yml": true, ".toml": true, ".xml": true, ".html": true, ".css": true, ".sql": true,
	}
	return plainExts[ext]
}

func readTextFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxExtractedTextBytes+1))
	if err != nil {
		return "", err
	}
	if !utf8.Valid(raw) {
		raw = bytes.ToValidUTF8(raw, []byte("�"))
	}
	return string(raw), nil
}

func extractDocxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	var docs []*zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" || strings.HasPrefix(f.Name, "word/header") || strings.HasPrefix(f.Name, "word/footer") {
			docs = append(docs, f)
		}
	}
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })
	var b strings.Builder
	for _, f := range docs {
		if b.Len() > maxExtractedTextBytes {
			break
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		decoder := xml.NewDecoder(io.LimitReader(rc, maxExtractedTextBytes+1))
		for {
			tok, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = rc.Close()
				return "", err
			}
			switch t := tok.(type) {
			case xml.CharData:
				text := strings.TrimSpace(string(t))
				if text != "" {
					if b.Len() > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(text)
				}
			case xml.StartElement:
				name := t.Name.Local
				if name == "p" || name == "br" || name == "tab" {
					b.WriteByte('\n')
				}
			}
			if b.Len() > maxExtractedTextBytes {
				break
			}
		}
		_ = rc.Close()
	}
	return normalizeExtractedText(b.String()), nil
}

func extractPDFTextBestEffort(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(raw) > 4<<20 {
		raw = raw[:4<<20]
	}
	matches := regexp.MustCompile(`\((?:\\.|[^\\)]){2,}\)`).FindAll(raw, 3000)
	var b strings.Builder
	for _, match := range matches {
		if b.Len() > maxExtractedTextBytes {
			break
		}
		text := decodePDFLiteralString(match[1 : len(match)-1])
		if looksLikeHumanText(text) {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(text)
		}
	}
	text := normalizeExtractedText(b.String())
	if text == "" {
		return "", nil
	}
	return text, nil
}

func decodePDFLiteralString(raw []byte) string {
	var out []byte
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch != '\\' || i+1 >= len(raw) {
			out = append(out, ch)
			continue
		}
		i++
		switch raw[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b', 'f':
		case '(', ')', '\\':
			out = append(out, raw[i])
		default:
			out = append(out, raw[i])
		}
	}
	out = bytes.ToValidUTF8(out, []byte(""))
	return html.UnescapeString(string(out))
}

func looksLikeHumanText(text string) bool {
	text = strings.TrimSpace(text)
	if len([]rune(text)) < 2 {
		return false
	}
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r) {
			letters++
		}
	}
	return letters >= 2
}

func normalizeExtractedText(text string) string {
	text = strings.ReplaceAll(text, "\x00", "")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(regexp.MustCompile(`[ \t]+`).ReplaceAllString(line, " "))
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/1024/1024/1024)
}
