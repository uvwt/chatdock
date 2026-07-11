package store

import (
	"testing"
	"time"
)

func TestMigrateToolEmbeddingBlobsRollsBackWhenMarkerWriteFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`DELETE FROM meta WHERE key = 'tool_embedding_blobs_migrated'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO tool_embeddings(workspace_id, full_name, source_hash, embedding_model, embedding_json, embedding_blob, indexed_at) VALUES(?, ?, ?, ?, ?, x'', ?)`, defaultWorkspaceID, "legacy_tool", "legacy-hash", "legacy-model", `[0.1,0.2]`, formatDBTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_embedding_blob_marker
BEFORE INSERT ON meta
WHEN NEW.key = 'tool_embedding_blobs_migrated'
BEGIN
  SELECT RAISE(ABORT, 'forced embedding marker failure');
END`); err != nil {
		t.Fatal(err)
	}

	if err := store.migrateToolEmbeddingBlobs(); err == nil {
		t.Fatal("expected tool embedding migration failure")
	}
	var blobBytes int
	if err := store.db.QueryRow(`SELECT length(embedding_blob) FROM tool_embeddings WHERE workspace_id = ? AND full_name = ?`, defaultWorkspaceID, "legacy_tool").Scan(&blobBytes); err != nil {
		t.Fatal(err)
	}
	if blobBytes != 0 {
		t.Fatalf("embedding blob survived migration rollback: %d bytes", blobBytes)
	}
	marker, err := store.metaValue("tool_embedding_blobs_migrated")
	if err != nil {
		t.Fatal(err)
	}
	if marker != "" {
		t.Fatalf("migration marker survived rollback: %q", marker)
	}
}
