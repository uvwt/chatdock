package model

// PinnedFeedResponse is the sidebar pin zone payload.
// Sessions, projects and scheduled tasks are returned together in one request.
type PinnedFeedResponse struct {
	Sessions []SessionSummary `json:"sessions"`
	Projects []ProjectSummary `json:"projects"`
	Tasks    []ScheduledTask  `json:"tasks"`
}
