package attachment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExtractAttachmentTextKeepsEmptyPlainTextStored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	text, status, err := ExtractText(path, "empty.txt", "text/plain")
	if err != nil || text != "" || status != "stored" {
		t.Fatalf("empty text extraction text=%q status=%q error=%v", text, status, err)
	}
}

func TestReadFilePrefixIsBoundedAndUTF8Safe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.txt")
	content := []byte(strings.Repeat("a", 31) + "你" + strings.Repeat("z", 64))
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, truncated, err := readFilePrefix(path, 32)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(raw) > 32 || !utf8.Valid(raw) {
		t.Fatalf("bounded prefix len=%d truncated=%v valid=%v raw=%q", len(raw), truncated, utf8.Valid(raw), raw)
	}
	if string(raw) != strings.Repeat("a", 31) {
		t.Fatalf("partial UTF-8 rune was not removed: %q", raw)
	}
}

func TestPDFExtractionReadsOnlyBoundedPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.pdf")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("%PDF-1.7\n(Hello PDF)\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Truncate(maxPDFProbeBytes + 32<<20); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := extractPDFTextBestEffort(path)
	if err != nil || !strings.Contains(text, "Hello PDF") {
		t.Fatalf("PDF extraction text=%q error=%v", text, err)
	}
}

func TestDecodePDFLiteralStringAndNormalization(t *testing.T) {
	decoded := decodePDFLiteralString([]byte(`Line\n\(demo\)&amp;`))
	if decoded != "Line\n(demo)&" {
		t.Fatalf("decoded PDF literal = %q", decoded)
	}
	if got := normalizeExtractedText(" one\t two \n\n three\x00 "); got != "one two\nthree" {
		t.Fatalf("normalized text = %q", got)
	}
	if !looksLikeHumanText("A1") || looksLikeHumanText("!") {
		t.Fatal("human text heuristic returned an unexpected result")
	}
}
