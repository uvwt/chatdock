package llm

import (
	"regexp"
	"strings"
)

var thinkBlockRegexp = regexp.MustCompile(`(?is)<think>.*?</think>`)

func StripThinkingContent(content string) string {
	content = thinkBlockRegexp.ReplaceAllString(content, "")
	lower := strings.ToLower(content)
	for {
		start := strings.Index(lower, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(lower[start:], "</think>")
		if end < 0 {
			content = content[:start]
			break
		}
		end = start + end + len("</think>")
		content = content[:start] + content[end:]
		lower = strings.ToLower(content)
	}
	return strings.TrimSpace(content)
}

type ThinkingFilter struct {
	enabled bool
	buffer  string
	hidden  bool
}

func NewThinkingFilter(hideThinking bool) *ThinkingFilter {
	return &ThinkingFilter{enabled: hideThinking}
}

func (f *ThinkingFilter) Push(delta string) string {
	if !f.enabled || delta == "" {
		return delta
	}

	f.buffer += delta
	var out strings.Builder

	for len(f.buffer) > 0 {
		lower := strings.ToLower(f.buffer)
		if f.hidden {
			end := strings.Index(lower, "</think>")
			if end < 0 {
				keep := min(len(f.buffer), len("</think>")-1)
				if keep > 0 {
					f.buffer = f.buffer[len(f.buffer)-keep:]
				} else {
					f.buffer = ""
				}
				return out.String()
			}
			f.buffer = f.buffer[end+len("</think>"):]
			f.hidden = false
			continue
		}

		start := strings.Index(lower, "<think>")
		if start < 0 {
			keep := commonPrefixSuffixKeep(f.buffer, "<think>")
			emitLen := len(f.buffer) - keep
			if emitLen > 0 {
				out.WriteString(f.buffer[:emitLen])
				f.buffer = f.buffer[emitLen:]
			}
			return out.String()
		}

		if start > 0 {
			out.WriteString(f.buffer[:start])
		}
		f.buffer = f.buffer[start+len("<think>"):]
		f.hidden = true
	}
	return out.String()
}

func (f *ThinkingFilter) Flush() string {
	if !f.enabled || !f.hidden {
		out := f.buffer
		f.buffer = ""
		return out
	}
	f.buffer = ""
	return ""
}

func commonPrefixSuffixKeep(s string, marker string) int {
	maxKeep := min(len(s), len(marker)-1)
	lowerS := strings.ToLower(s)
	lowerMarker := strings.ToLower(marker)
	for keep := maxKeep; keep > 0; keep-- {
		if strings.HasSuffix(lowerS, lowerMarker[:keep]) {
			return keep
		}
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
