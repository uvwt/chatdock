package store

import (
	"testing"

	"chatdock/internal/chatdock/model"
)

func TestDeleteProjectDetachesSessions(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	project, err := store.CreateProject(model.CreateProjectRequest{Name: "研究", Prompt: "只做研究总结"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := store.DeleteProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("project was not deleted")
	}

	detached, ok, err := store.GetSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("session was deleted with project")
	}
	if detached.ProjectID != "" {
		t.Fatalf("session project_id = %q, want empty", detached.ProjectID)
	}
	assertSQLiteForeignKeysClean(t, store)
}

func TestSessionProjectFilters(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	project, err := store.CreateProject(model.CreateProjectRequest{Name: "项目"})
	if err != nil {
		t.Fatal(err)
	}
	projectSession, err := store.CreateSession(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	plainSession, err := store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}

	all, err := store.ListSessions(SessionProjectFilter{Mode: SessionProjectFilterAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all sessions = %#v", all)
	}
	projectOnly, err := store.ListSessions(SessionProjectFilter{Mode: SessionProjectFilterByProject, ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(projectOnly) != 1 || projectOnly[0].ID != projectSession.ID {
		t.Fatalf("project sessions = %#v", projectOnly)
	}
	plainOnly, err := store.ListSessions(SessionProjectFilter{Mode: SessionProjectFilterNoProject})
	if err != nil {
		t.Fatal(err)
	}
	if len(plainOnly) != 1 || plainOnly[0].ID != plainSession.ID {
		t.Fatalf("plain sessions = %#v", plainOnly)
	}
}

func assertSQLiteForeignKeysClean(t *testing.T, store *Store) {
	t.Helper()
	rows, err := store.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check returned violations")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
