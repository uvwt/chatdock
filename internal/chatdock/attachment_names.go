package chatdock

import (
	"path/filepath"
	"strings"
	"unicode"
)

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
