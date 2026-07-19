package chatdock

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseOptionalIntAppliesDefaultsAndBounds(t *testing.T) {
	if got, err := parseOptionalInt("", 30, 1, 100, "limit"); err != nil || got != 30 {
		t.Fatalf("default limit = %d, %v", got, err)
	}
	if got, err := parseOptionalInt(" 100 ", 30, 1, 100, "limit"); err != nil || got != 100 {
		t.Fatalf("parsed limit = %d, %v", got, err)
	}
	for _, raw := range []string{"bad", "0", "101"} {
		if _, err := parseOptionalInt(raw, 30, 1, 100, "limit"); err == nil {
			t.Fatalf("invalid limit %q was accepted", raw)
		}
	}
}

func TestChatJobEventCursorUsesQueryThenLastEventID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/chat/jobs/job/events", nil)
	request.Header.Set("Last-Event-ID", "12")
	if got, err := chatJobEventCursor(request); err != nil || got != 12 {
		t.Fatalf("header cursor = %d, %v", got, err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/chat/jobs/job/events?after=7", nil)
	request.Header.Set("Last-Event-ID", "12")
	if got, err := chatJobEventCursor(request); err != nil || got != 7 {
		t.Fatalf("query cursor = %d, %v", got, err)
	}
	for _, raw := range []string{"-1", "invalid", "2147483648"} {
		request = httptest.NewRequest(http.MethodGet, "/api/chat/jobs/job/events?after="+raw, nil)
		if _, err := chatJobEventCursor(request); err == nil {
			t.Fatalf("invalid cursor %q was accepted", raw)
		}
	}
}

func TestInvalidScheduledRunLimitReturnsBadRequestBeforeStoreAccess(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/scheduled-tasks/task/runs?limit=invalid", nil)
	request.SetPathValue("id", "task")
	response := httptest.NewRecorder()
	(&App{}).handleListScheduledTaskRuns(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestInvalidChatJobCursorReturnsBadRequestBeforeStreaming(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/chat/jobs/job/events?after=-1", nil)
	request.SetPathValue("id", "job")
	response := httptest.NewRecorder()
	(&App{}).handleChatJobEvents(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
