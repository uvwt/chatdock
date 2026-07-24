package store

import (
	"fmt"
	"testing"
	"time"

	"chatdock/internal/model"
)

func createChatJobForTest(t *testing.T, store *Store, sessionID string, requestID string) (ChatJob, error) {
	t.Helper()
	if store == nil {
		return ChatJob{}, fmt.Errorf("store is nil")
	}
	if sessionID == "" {
		return ChatJob{}, fmt.Errorf("session id is required")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	job := newChatJob(sessionID, requestID, time.Now())
	if err := insertChatJobWith(store.db, job); err != nil {
		return ChatJob{}, err
	}
	return job, nil
}

func appendUserMessageForTest(t *testing.T, store *Store, sessionID string, content string) *model.Session {
	t.Helper()
	session, _, _, err := store.PrepareChat(model.ChatRequest{SessionID: sessionID, Message: content})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func appendUserMessageWithAttachmentsForTest(store *Store, sessionID string, content string, attachmentIDs []string) (*model.Session, error) {
	session, _, _, err := store.PrepareChat(model.ChatRequest{SessionID: sessionID, Message: content, AttachmentIDs: attachmentIDs})
	return session, err
}
