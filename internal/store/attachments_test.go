package store

import (
	"testing"
	"time"

	"chatdock/internal/model"
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
	if _, err := store.SaveAttachment(first); err != nil {
		t.Fatal(err)
	}

	duplicateID := first
	duplicateID.Name = "second.txt"
	duplicateID.StoragePath = "/tmp/second.txt"
	duplicateID.SHA256 = "sha-second"
	if _, err := store.SaveAttachment(duplicateID); err == nil {
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

func TestSaveAttachmentUsesCanonicalBlobPath(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	first := model.AttachmentRecord{
		Attachment:  model.Attachment{ID: "canonical-a", Name: "a.txt", MIMEType: "text/plain", Size: 4, Status: "stored", CreatedAt: now},
		StoragePath: "/tmp/canonical-a.txt",
		SHA256:      "canonical-sha",
	}
	savedFirst, err := store.SaveAttachment(first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "canonical-b"
	second.Name = "b.txt"
	second.StoragePath = "/tmp/canonical-b.txt"
	savedSecond, err := store.SaveAttachment(second)
	if err != nil {
		t.Fatal(err)
	}
	if savedSecond.StoragePath != savedFirst.StoragePath {
		t.Fatalf("duplicate SHA path = %q, want canonical %q", savedSecond.StoragePath, savedFirst.StoragePath)
	}
	loaded, err := store.AttachmentRecordByID(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StoragePath != savedFirst.StoragePath {
		t.Fatalf("stored duplicate path = %q, want canonical %q", loaded.StoragePath, savedFirst.StoragePath)
	}
	blob, ok, err := store.AttachmentBlobBySHA256(first.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || blob.RefCount != 2 {
		t.Fatalf("unexpected canonical blob: %#v", blob)
	}
}
