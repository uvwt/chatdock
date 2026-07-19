package store

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeAttachmentIDsDeduplicatesBeforeApplyingLimit(t *testing.T) {
	ids := make([]string, 0, maxAttachmentsPerMessage*2)
	for i := 0; i < maxAttachmentsPerMessage; i++ {
		id := fmt.Sprintf("attachment-%d", i)
		ids = append(ids, " "+id+" ", id)
	}
	normalized, err := normalizeAttachmentIDs(ids)
	if err != nil || len(normalized) != maxAttachmentsPerMessage {
		t.Fatalf("normalized attachment IDs = %d, error=%v", len(normalized), err)
	}
}

func TestAttachmentRecordsRejectTooManyIDsBeforeSQL(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ids := make([]string, maxAttachmentsPerMessage+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("attachment-%d", i)
	}
	store.mu.Lock()
	_, err = store.attachmentRecordsByIDsLocked(defaultWorkspaceID, ids)
	store.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "attachment count exceeds 20") {
		t.Fatalf("attachment limit error = %v", err)
	}
}
