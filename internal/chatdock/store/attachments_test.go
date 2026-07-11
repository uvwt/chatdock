package store

import (
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestSaveAttachmentRollsBackBlobWhenAttachmentInsertFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first := model.AttachmentRecord{
		Attachment: model.Attachment{
			ID:        "attachment-a",
			Name:      "first.txt",
			MIMEType:  "text/plain",
			Size:      5,
			Status:    "stored",
			CreatedAt: time.Now(),
		},
		StoragePath: "/tmp/first.txt",
		SHA256:      "sha-first",
	}
	if err := store.SaveAttachment("default", first); err != nil {
		t.Fatal(err)
	}

	duplicateID := first
	duplicateID.Name = "second.txt"
	duplicateID.StoragePath = "/tmp/second.txt"
	duplicateID.SHA256 = "sha-second"
	if err := store.SaveAttachment("default", duplicateID); err == nil {
		t.Fatal("expected duplicate attachment id to fail")
	}
	if _, ok, err := store.AttachmentBlobBySHA256("sha-second"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("failed attachment insert must roll back its blob row")
	}
	blob, ok, err := store.AttachmentBlobBySHA256("sha-first")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || blob.RefCount != 1 {
		t.Fatalf("unexpected persisted blob after rollback: %#v", blob)
	}
}
