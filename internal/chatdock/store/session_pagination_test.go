package store

import (
	"reflect"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestListSessionPageKeepsStableOrderAcrossPages(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	type fixture struct {
		title     string
		pinned    bool
		updatedAt string
	}
	fixtures := []fixture{
		{title: "普通最旧", updatedAt: "2026-07-21T08:00:00.000000000Z"},
		{title: "置顶较新", pinned: true, updatedAt: "2026-07-21T12:00:00.000000000Z"},
		{title: "普通最新", updatedAt: "2026-07-21T10:00:00.000000000Z"},
		{title: "置顶较旧", pinned: true, updatedAt: "2026-07-21T11:00:00.000000000Z"},
		{title: "普通居中", updatedAt: "2026-07-21T09:00:00.000000000Z"},
	}
	for _, item := range fixtures {
		session, err := store.CreateSession("")
		if err != nil {
			t.Fatal(err)
		}
		appendUserMessageForTest(t, store, session.ID, "摘要 "+item.title)
		if _, err := store.db.Exec(`UPDATE sessions SET title = ?, pinned = ?, updated_at = ? WHERE id = ?`, item.title, boolInt(item.pinned), item.updatedAt, session.ID); err != nil {
			t.Fatal(err)
		}
	}

	var titles []string
	cursor := ""
	for pageNumber := 0; ; pageNumber++ {
		items, nextCursor, hasMore, err := store.ListSessionPage(SessionProjectFilter{Mode: SessionProjectFilterAll}, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 0 {
			t.Fatalf("page %d unexpectedly empty", pageNumber)
		}
		for _, item := range items {
			titles = append(titles, item.Title)
			if item.Count != 1 || item.LastRole != "user" || item.Preview == "" {
				t.Fatalf("summary missing message data: %#v", item)
			}
		}
		if !hasMore {
			break
		}
		if nextCursor == "" || nextCursor == cursor {
			t.Fatalf("page %d returned invalid cursor %q", pageNumber, nextCursor)
		}
		cursor = nextCursor
	}

	want := []string{"置顶较新", "置顶较旧", "普通最新", "普通居中", "普通最旧"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("page order = %#v, want %#v", titles, want)
	}
	if _, _, _, err := store.ListSessionPage(SessionProjectFilter{Mode: SessionProjectFilterAll}, "invalid", 2); err == nil {
		t.Fatal("invalid cursor should fail")
	}
}

func TestSearchSessionPageDoesNotRepeatResults(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for index, title := range []string{"搜索最新", "搜索居中", "搜索最旧"} {
		session, err := store.CreateSession("")
		if err != nil {
			t.Fatal(err)
		}
		appendUserMessageForTest(t, store, session.ID, "共同分页关键词 "+title)
		updatedAt := time.Date(2026, 7, 21, 10-index, 0, 0, index+1, time.UTC).Format(time.RFC3339Nano)
		if _, err := store.db.Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`, title, updatedAt, session.ID); err != nil {
			t.Fatal(err)
		}
	}

	first, cursor, hasMore, err := store.SearchSessionPage("共同分页关键词", SessionProjectFilter{Mode: SessionProjectFilterAll}, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || !hasMore || cursor == "" {
		t.Fatalf("unexpected first page: items=%#v cursor=%q hasMore=%v", first, cursor, hasMore)
	}
	second, nextCursor, hasMore, err := store.SearchSessionPage("共同分页关键词", SessionProjectFilter{Mode: SessionProjectFilterAll}, cursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || hasMore || nextCursor != "" {
		t.Fatalf("unexpected second page: items=%#v cursor=%q hasMore=%v", second, nextCursor, hasMore)
	}
	seen := map[string]bool{}
	for _, item := range append(first, second...) {
		if seen[item.ID] {
			t.Fatalf("duplicate search result %s", item.ID)
		}
		seen[item.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("search returned %d unique results", len(seen))
	}
	if _, _, _, err := store.SearchSessionPage("共同分页关键词", SessionProjectFilter{Mode: SessionProjectFilterAll}, "bad", 2); err == nil {
		t.Fatal("invalid search cursor should fail")
	}
}

func TestSearchSessionPageFiltersByProject(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	project, err := store.CreateProject(model.CreateProjectRequest{Name: "项目", Prompt: "项目提示词"})
	if err != nil {
		t.Fatal(err)
	}
	projectSession, err := store.CreateSession(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	appendUserMessageForTest(t, store, projectSession.ID, "过滤关键词")
	plainSession, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	appendUserMessageForTest(t, store, plainSession.ID, "过滤关键词")

	projectOnly, _, _, err := store.SearchSessionPage("过滤关键词", SessionProjectFilter{Mode: SessionProjectFilterByProject, ProjectID: project.ID}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(projectOnly) != 1 || projectOnly[0].ID != projectSession.ID {
		t.Fatalf("project search results = %#v, want only %s", projectOnly, projectSession.ID)
	}
	plainOnly, _, _, err := store.SearchSessionPage("过滤关键词", SessionProjectFilter{Mode: SessionProjectFilterNoProject}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(plainOnly) != 1 || plainOnly[0].ID != plainSession.ID {
		t.Fatalf("plain search results = %#v, want only %s", plainOnly, plainSession.ID)
	}
}

func TestSessionSummaryPreviewUsesLastMessageError(t *testing.T) {
	role, preview := sessionSummaryPreview("assistant", "", `{"message":"模型暂时不可用"}`)
	if role != "assistant" || preview != "模型暂时不可用" {
		t.Fatalf("unexpected preview role=%q preview=%q", role, preview)
	}
}

func TestSessionPaginationIndexExists(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_sessions_project_updated'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("pagination index count = %d", count)
	}
}
