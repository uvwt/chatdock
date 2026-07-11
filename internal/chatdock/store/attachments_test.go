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
	if _, err := store.SaveAttachment("default", first); err != nil {
		t.Fatal(err)
	}

	duplicateID := first
	duplicateID.Name = "second.txt"
	duplicateID.StoragePath = "/tmp/second.txt"
	duplicateID.SHA256 = "sha-second"
	if _, err := store.SaveAttachment("default", duplicateID); err == nil {
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

func TestMigrateAttachmentBlobsRollsBackWhenMarkerWriteFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`DELETE FROM meta WHERE key = 'attachment_blobs_migrated'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DELETE FROM attachment_blobs`); err != nil {
		t.Fatal(err)
	}
	now := formatDBTime(time.Now())
	if _, err := store.db.Exec(`INSERT INTO attachments(workspace_id, id, session_id, message_id, filename, mime_type, size, storage_path, sha256, text_content, status, created_at) VALUES(?, ?, '', '', ?, ?, ?, ?, ?, '', 'stored', ?)`, defaultWorkspaceID, "legacy-attachment", "legacy.txt", "text/plain", 6, "/tmp/legacy.txt", "legacy-sha", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_attachment_blob_marker
BEFORE INSERT ON meta
WHEN NEW.key = 'attachment_blobs_migrated'
BEGIN
  SELECT RAISE(ABORT, 'forced attachment marker failure');
END`); err != nil {
		t.Fatal(err)
	}

	if err := store.migrateAttachmentBlobs(); err == nil {
		t.Fatal("expected attachment blob migration failure")
	}
	var blobCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM attachment_blobs WHERE sha256 = 'legacy-sha'`).Scan(&blobCount); err != nil {
		t.Fatal(err)
	}
	if blobCount != 0 {
		t.Fatalf("attachment blob survived migration rollback: %d", blobCount)
	}
	marker, err := store.metaValue("attachment_blobs_migrated")
	if err != nil {
		t.Fatal(err)
	}
	if marker != "" {
		t.Fatalf("migration marker survived rollback: %q", marker)
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
	savedFirst, err := store.SaveAttachment(defaultWorkspaceID, first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "canonical-b"
	second.Name = "b.txt"
	second.StoragePath = "/tmp/canonical-b.txt"
	savedSecond, err := store.SaveAttachment(defaultWorkspaceID, second)
	if err != nil {
		t.Fatal(err)
	}
	if savedSecond.StoragePath != savedFirst.StoragePath {
		t.Fatalf("duplicate SHA path = %q, want canonical %q", savedSecond.StoragePath, savedFirst.StoragePath)
	}
	loaded, err := store.AttachmentRecordByID(defaultWorkspaceID, second.ID)
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

func TestSaveAttachmentReplacesZeroReferenceBlobPath(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateWorkspace(model.CreateWorkspaceRequest{Name: "cache-source"}); err != nil {
		t.Fatal(err)
	}
	first := model.AttachmentRecord{
		Attachment:  model.Attachment{ID: "zero-ref-a", Name: "a.txt", MIMEType: "text/plain", Size: 4, Status: "stored", CreatedAt: time.Now()},
		StoragePath: "/tmp/zero-ref-old.txt",
		SHA256:      "zero-ref-sha",
	}
	if _, err := store.SaveAttachment("cache-source", first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteWorkspace(model.WorkspaceIDRequest{Name: "cache-source"}); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = "zero-ref-b"
	second.StoragePath = "/tmp/zero-ref-new.txt"
	saved, err := store.SaveAttachment(defaultWorkspaceID, second)
	if err != nil {
		t.Fatal(err)
	}
	if saved.StoragePath != second.StoragePath {
		t.Fatalf("zero-ref blob path = %q, want replacement %q", saved.StoragePath, second.StoragePath)
	}
	blob, ok, err := store.AttachmentBlobBySHA256(second.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || blob.RefCount != 1 || blob.StoragePath != second.StoragePath {
		t.Fatalf("unexpected replaced blob: %#v", blob)
	}
}
