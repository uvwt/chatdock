package store

import "testing"

func TestInitializeSetupRollsBackAllRecordsWhenMCPWriteFails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	beforeMeta, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_setup_mcp_write
BEFORE INSERT ON workspace_kv
WHEN NEW.workspace_id = 'onboarding' AND NEW.key = 'mcp'
BEGIN
  SELECT RAISE(ABORT, 'forced setup mcp failure');
END`); err != nil {
		t.Fatal(err)
	}

	_, err = store.InitializeSetup(SetupInitRequest{
		WorkspaceName: "onboarding",
		BaseURL:       "https://example.test/v1",
		APIKey:        "setup-secret",
		Model:         "setup-model",
	})
	if err == nil {
		t.Fatal("expected setup failure")
	}
	assertWorkspaceAbsent(t, store, "onboarding")
	afterMeta, err := store.metaValue(modelProvidersMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	if afterMeta != beforeMeta {
		t.Fatal("setup provider metadata survived transaction rollback")
	}
}
