package chatdock

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxExtractedTextBytes = 180 << 10
	maxPDFProbeBytes      = 4 << 20
)

func extractAttachmentText(path string, name string, mimeType string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	lowerMime := strings.ToLower(strings.Split(mimeType, ";")[0])
	switch {
	case isPlainTextAttachment(ext, lowerMime):
		text, err := readTextFile(path)
		return text, statusForText(text), err
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
	raw, _, err := readFilePrefix(path, maxExtractedTextBytes)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(raw) {
		raw = bytes.ToValidUTF8(raw, []byte("�"))
	}
	return string(raw), nil
}

func readFilePrefix(path string, maxBytes int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	truncated := int64(len(raw)) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
		for len(raw) > 0 && !utf8.Valid(raw) {
			raw = raw[:len(raw)-1]
		}
	}
	return raw, truncated, nil
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
	raw, _, err := readFilePrefix(path, maxPDFProbeBytes)
	if err != nil {
		return "", err
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
