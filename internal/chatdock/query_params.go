package chatdock

import (
	"net/http"
	"strings"
)

func chatJobEventCursor(r *http.Request) (int, error) {
	rawAfter := r.URL.Query().Get("after")
	if strings.TrimSpace(rawAfter) == "" {
		rawAfter = r.Header.Get("Last-Event-ID")
	}
	return parseOptionalInt(rawAfter, 0, 0, 2_147_483_647, "after")
}
