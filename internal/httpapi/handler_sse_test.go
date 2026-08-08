package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteSSEWithIDIncludesResumeSequence(t *testing.T) {
	recorder := httptest.NewRecorder()
	if err := writeSSEWithID(recorder, recorder, 12, "delta", map[string]string{"content": "继续输出"}); err != nil {
		t.Fatal(err)
	}

	body := recorder.Body.String()
	for _, expected := range []string{"id: 12\n", "event: delta\n", `data: {"content":"继续输出"}` + "\n\n"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("SSE body missing %q: %q", expected, body)
		}
	}
	if !recorder.Flushed {
		t.Fatal("SSE event was not flushed")
	}
}

func TestWriteSSEHeartbeatUsesCommentFrame(t *testing.T) {
	recorder := httptest.NewRecorder()
	if err := writeSSEHeartbeat(recorder, recorder); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Body.String(); got != ": heartbeat\n\n" {
		t.Fatalf("unexpected heartbeat frame: %q", got)
	}
	if !recorder.Flushed {
		t.Fatal("SSE heartbeat was not flushed")
	}
}
