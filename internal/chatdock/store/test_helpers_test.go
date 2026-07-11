package store

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func createChatJobForTest(t *testing.T, store *Store, workspaceID string, sessionID string, requestID string) (ChatJob, error) {
	t.Helper()
	if store == nil {
		return ChatJob{}, fmt.Errorf("store is nil")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	sessionID = strings.TrimSpace(sessionID)
	if workspaceID == "" || sessionID == "" {
		return ChatJob{}, fmt.Errorf("workspace id and session id are required")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, err := store.requireWorkspaceLocked(workspaceID); err != nil {
		return ChatJob{}, err
	}
	job := newChatJob(workspaceID, sessionID, requestID, time.Now())
	if err := insertChatJobWith(store.db, job); err != nil {
		return ChatJob{}, err
	}
	return job, nil
}

func appendUserMessageForTest(t *testing.T, store *Store, workspaceID string, sessionID string, content string) *model.Session {
	t.Helper()
	session, _, _, err := store.PrepareChat(workspaceID, model.ChatRequest{SessionID: sessionID, Message: content})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func appendUserMessageWithAttachmentsForTest(store *Store, workspaceID string, sessionID string, content string, attachmentIDs []string) (*model.Session, error) {
	session, _, _, err := store.PrepareChat(workspaceID, model.ChatRequest{SessionID: sessionID, Message: content, AttachmentIDs: attachmentIDs})
	return session, err
}
