package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chatdock/internal/model"
)

type chatJobGuidance struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type guideChatJobRequest struct {
	Message string `json:"message"`
}

func (a *Server) handleGuideChatJob(w http.ResponseWriter, r *http.Request) {
	var input guideChatJobRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.enqueueChatJobGuidance(r.PathValue("id"), input.Message)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]any{"guidance": item})
}

func (a *Server) enqueueChatJobGuidance(jobID string, message string) (chatJobGuidance, error) {
	jobID = strings.TrimSpace(jobID)
	message = strings.TrimSpace(message)
	if jobID == "" {
		return chatJobGuidance{}, fmt.Errorf("job id is empty")
	}
	if message == "" {
		return chatJobGuidance{}, fmt.Errorf("guidance message is empty")
	}
	runes := []rune(message)
	if len(runes) > 4000 {
		message = string(runes[:4000])
	}
	job, err := a.store.GetChatJob(jobID)
	if err != nil {
		return chatJobGuidance{}, err
	}
	if job.Status != "running" {
		return chatJobGuidance{}, fmt.Errorf("chat job is not running")
	}
	item := chatJobGuidance{ID: model.NewID(), JobID: jobID, Message: message, CreatedAt: time.Now()}
	if _, err := a.store.AddChatJobEvent(jobID, "guidance_queued", item); err != nil {
		return chatJobGuidance{}, err
	}
	a.jobMu.Lock()
	a.jobGuidance[jobID] = append(a.jobGuidance[jobID], item)
	a.jobMu.Unlock()
	return item, nil
}

func (a *Server) drainChatJobGuidance(jobID string) []chatJobGuidance {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}
	a.jobMu.Lock()
	items := append([]chatJobGuidance(nil), a.jobGuidance[jobID]...)
	delete(a.jobGuidance, jobID)
	a.jobMu.Unlock()
	return items
}

func (a *Server) clearChatJobGuidance(jobID string) {
	a.jobMu.Lock()
	delete(a.jobGuidance, strings.TrimSpace(jobID))
	a.jobMu.Unlock()
}
