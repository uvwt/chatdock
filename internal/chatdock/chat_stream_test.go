package chatdock

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestChatStreamPublishesFirstDeltaBeforeModelCompletes(t *testing.T) {
	releaseModel := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseModel) }) }
	defer release()

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected model path", http.StatusNotFound)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"O\"}}]}\n\n")
		flusher.Flush()
		<-releaseModel
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"K\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer modelServer.Close()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if _, err := app.store.SaveModelConfig("default", model.ModelConfig{BaseURL: modelServer.URL, Model: "demo"}); err != nil {
		t.Fatal(err)
	}
	session, err := app.store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.RenameSession("default", session.ID, "固定标题"); err != nil {
		t.Fatal(err)
	}

	chatServer := httptest.NewServer(app.routes())
	defer chatServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body := bytes.NewBufferString(`{"session_id":"` + session.ID + `","message":"只回复 OK"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatServer.URL+"/api/chat/stream", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := chatServer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected chat stream status: %s", resp.Status)
	}

	reader := bufio.NewReader(resp.Body)
	firstDelta := make(chan error, 1)
	go func() { firstDelta <- waitForSSEEvent(reader, "delta") }()
	select {
	case err := <-firstDelta:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first delta was not published while the model stream was still open")
	}

	release()
	messageEnd := make(chan error, 1)
	go func() { messageEnd <- waitForSSEEvent(reader, "message_end") }()
	select {
	case err := <-messageEnd:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("message_end was not published after the model completed")
	}
}

func waitForSSEEvent(reader *bufio.Reader, want string) error {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimSpace(line) == "event: "+want {
			return nil
		}
	}
}
