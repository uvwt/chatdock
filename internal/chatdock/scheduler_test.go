package chatdock

import "testing"

func TestStartScheduledTaskRejectsMissingWorkspaceOrTaskID(t *testing.T) {
	app := &App{}
	for _, tc := range []struct {
		workspaceID string
		taskID      string
	}{
		{},
		{workspaceID: "default"},
		{taskID: "task-1"},
		{workspaceID: "   ", taskID: "task-1"},
	} {
		app.startScheduledTask(tc.workspaceID, tc.taskID, false)
	}
}
