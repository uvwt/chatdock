package chatdock

import "testing"

func TestStartScheduledTaskRejectsMissingTaskID(t *testing.T) {
	app := &App{}
	for _, taskID := range []string{"", "   "} {
		app.startScheduledTask(taskID, false)
	}
}
