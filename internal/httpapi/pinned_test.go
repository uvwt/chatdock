package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatdock/internal/model"
)

func TestPinnedFeedAPI(t *testing.T) {
	app, err := NewServer(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}

	session, err := app.store.CreateSession("")
	if err != nil {
		t.Fatal(err)
	}
	appendUserMessageForAppTest(t, app, session.ID, "置顶会话")
	if _, err := app.store.PinSession(session.ID, true); err != nil {
		t.Fatal(err)
	}

	project, err := app.store.CreateProject(model.CreateProjectRequest{Name: "置顶项目"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.PinProject(project.ID, true); err != nil {
		t.Fatal(err)
	}

	taskResp, err := app.store.CreateScheduledTask(model.ScheduledTaskRequest{
		Title:           "置顶任务",
		Prompt:          "run",
		Enabled:         true,
		ScheduleType:    "interval",
		IntervalMinutes: 15,
		ContextMode:     model.ScheduledTaskContextStateless,
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := taskResp.Tasks[len(taskResp.Tasks)-1].ID
	if _, err := app.store.PinScheduledTask(taskID, true); err != nil {
		t.Fatal(err)
	}

	routes := app.routes()
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/pinned", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	var feed model.PinnedFeedResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Sessions) != 1 || feed.Sessions[0].ID != session.ID {
		t.Fatalf("sessions = %#v", feed.Sessions)
	}
	if len(feed.Projects) != 1 || feed.Projects[0].ID != project.ID {
		t.Fatalf("projects = %#v", feed.Projects)
	}
	if len(feed.Tasks) != 1 || feed.Tasks[0].ID != taskID {
		t.Fatalf("tasks = %#v", feed.Tasks)
	}
}
