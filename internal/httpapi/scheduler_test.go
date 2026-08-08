package httpapi

import "testing"

func TestStartScheduledTaskRejectsMissingTaskID(t *testing.T) {
	app := &Server{}
	for _, taskID := range []string{"", "   "} {
		app.startScheduledTask(taskID, false)
	}
}
