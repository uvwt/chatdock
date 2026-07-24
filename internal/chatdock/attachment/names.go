package attachment

import (
	"path/filepath"
	"strings"
)

func CleanUploadName(name string) string {
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
