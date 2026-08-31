package store

import (
	"testing"

	"chatdock/internal/model"
)

func TestSessionToolWorkingSetPersistsUsageAndCascadesWithSession(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"第一问", "第二问", "第三问"} {
		if _, _, _, err := store.PrepareChat(model.ChatRequest{SessionID: session.ID, Message: message}); err != nil {
			t.Fatal(err)
		}
	}

	entry := SessionToolWorkingSetEntry{ToolName: "github__issue_read", ResourceID: "github"}
	if err := store.RecordSessionToolDiscovery(session.ID, 2, []SessionToolWorkingSetEntry{entry}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSessionToolCall(session.ID, 3, entry); err != nil {
		t.Fatal(err)
	}
	// 较旧的并发写不能把最新 turn 倒退。
	if err := store.RecordSessionToolCall(session.ID, 1, entry); err != nil {
		t.Fatal(err)
	}

	entries, err := store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].LastDiscoveredTurn != 2 || entries[0].LastCalledTurn != 3 {
		t.Fatalf("unexpected working set: %#v", entries)
	}
	if ok, err := store.DeleteSession(session.ID); err != nil || !ok {
		t.Fatalf("delete session: ok=%v err=%v", ok, err)
	}
	entries, err = store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("session delete must cascade working set rows: %#v", entries)
	}
}

func TestEditUserMessageClearsSessionToolWorkingSet(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	prepared, _, history, err := store.PrepareChat(model.ChatRequest{SessionID: session.ID, Message: "第一问"})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("unexpected history: %#v", history)
	}
	entry := SessionToolWorkingSetEntry{ToolName: "github__issue_read", ResourceID: "github"}
	if err := store.RecordSessionToolCall(session.ID, 1, entry); err != nil {
		t.Fatal(err)
	}

	index := 0
	if _, err := store.EditUserMessageAndTruncate(session.ID, prepared.Messages[0].ID, &index, "改写后的第一问"); err != nil {
		t.Fatal(err)
	}
	entries, err := store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("edited timeline must clear working set: %#v", entries)
	}
}

func TestDeleteSessionToolWorkingSetEntriesIfUnchangedPreservesNewerTurn(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"第一问", "第二问"} {
		if _, _, _, err := store.PrepareChat(model.ChatRequest{SessionID: session.ID, Message: message}); err != nil {
			t.Fatal(err)
		}
	}
	entry := SessionToolWorkingSetEntry{ToolName: "github__issue_read", ResourceID: "github"}
	if err := store.RecordSessionToolDiscovery(session.ID, 1, []SessionToolWorkingSetEntry{entry}); err != nil {
		t.Fatal(err)
	}
	observed, err := store.SessionToolWorkingSet(session.ID)
	if err != nil || len(observed) != 1 {
		t.Fatalf("read observed working set: entries=%#v err=%v", observed, err)
	}

	// 较新的并发请求已经把同一工具推进到 turn 2，旧请求的清理不能删掉它。
	if err := store.RecordSessionToolCall(session.ID, 2, entry); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSessionToolWorkingSetEntriesIfUnchanged(session.ID, observed); err != nil {
		t.Fatal(err)
	}
	current, err := store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].LastCalledTurn != 2 {
		t.Fatalf("older cleanup removed newer working-set state: %#v", current)
	}

	if err := store.DeleteSessionToolWorkingSetEntriesIfUnchanged(session.ID, current); err != nil {
		t.Fatal(err)
	}
	current, err = store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 0 {
		t.Fatalf("unchanged entry should be deleted: %#v", current)
	}
}

func TestSessionToolWorkingSetRejectsWriteFromTruncatedFutureTurn(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	prepared, _, _, err := store.PrepareChat(model.ChatRequest{SessionID: session.ID, Message: "保留下来的第一问"})
	if err != nil {
		t.Fatal(err)
	}
	index := 0
	if _, err := store.EditUserMessageAndTruncate(session.ID, prepared.Messages[0].ID, &index, "改写后的第一问"); err != nil {
		t.Fatal(err)
	}

	// 模拟被截断时间线里的旧 job 在取消后仍晚到一个 turn 5 的工具结果。
	entry := SessionToolWorkingSetEntry{ToolName: "github__issue_read", ResourceID: "github"}
	if err := store.RecordSessionToolCall(session.ID, 5, entry); err != nil {
		t.Fatal(err)
	}
	entries, err := store.SessionToolWorkingSet(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("future turn from truncated timeline must not be persisted: %#v", entries)
	}
}
