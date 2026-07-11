package chatdock

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatdock/internal/chatdock/model"
)

func TestAppCloseCancelsAndWaitsForRegisteredJobs(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	if err := app.reserveChatJob(); err != nil {
		t.Fatal(err)
	}
	app.registerChatJobCancel("job-1", cancel)
	workerDone := make(chan struct{})
	go func() {
		<-jobCtx.Done()
		app.unregisterChatJobCancel("job-1")
		app.backgroundWG.Done()
		close(workerDone)
	}()

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close returned without waiting for the registered job")
	}

	lateCtx, lateCancel := context.WithCancel(context.Background())
	defer lateCancel()
	if err := app.reserveChatJob(); err == nil {
		app.backgroundWG.Done()
		t.Fatal("closing app must reject new background jobs")
	}
	if err := app.Close(); err != nil {
		t.Fatalf("second Close must be idempotent: %v", err)
	}
	select {
	case <-lateCtx.Done():
		t.Fatal("rejected registration must not take ownership of the caller's cancel function")
	default:
	}
}

func TestAppShutdownStopsHTTPServerAndClosesResources(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- app.server.Serve(listener)
	}()

	response, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status before shutdown: %d", response.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := app.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("unexpected Serve result: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP server did not stop")
	}
	if err := app.Close(); err != nil {
		t.Fatalf("Close after Shutdown must be idempotent: %v", err)
	}
}

func TestStartChatJobRejectsShutdownBeforePersistingMessage(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
	})
	session, err := app.store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	app.beginShutdown()

	if _, _, err := app.startChatJob(context.Background(), "default", model.ChatRequest{SessionID: session.ID, Message: "不应写入"}); err == nil {
		t.Fatal("expected shutdown rejection")
	}
	loaded, ok, err := app.store.GetSession("default", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(loaded.Messages) != 0 {
		t.Fatalf("shutdown-rejected message persisted: %#v", loaded)
	}
	jobs, err := app.store.ListChatJobs("default", session.ID, false, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("shutdown-rejected job persisted: %#v", jobs)
	}
}

func TestAppCloseCancelsAndWaitsForSessionTitleGeneration(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	releaseUpstream := make(chan struct{})
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStarted <- struct{}{}
		<-releaseUpstream
	}))
	defer func() {
		close(releaseUpstream)
		modelServer.Close()
	}()

	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := app.store.CreateSession("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := app.store.PrepareChat("default", model.ChatRequest{SessionID: session.ID, Message: "需要标题的问题"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.AppendAssistantMessage("default", session.ID, "这是回答"); err != nil {
		t.Fatal(err)
	}
	cfg := model.ModelConfig{BaseURL: modelServer.URL + "/v1", Model: "title-model"}
	app.startSessionTitleGeneration("req-title", "default", session.ID, cfg)

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("title request did not start")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- app.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("app close did not cancel and wait for title generation")
	}
}

func TestAppCloseCancelsAndWaitsForLifecycleWorker(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	workerStarted := make(chan struct{})
	workerStopped := make(chan struct{})
	if !app.startBackgroundWork(func(ctx context.Context) {
		close(workerStarted)
		<-ctx.Done()
		close(workerStopped)
	}) {
		t.Fatal("lifecycle worker was not started")
	}
	<-workerStarted
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-workerStopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not wait for lifecycle worker")
	}
	if app.startBackgroundWork(func(context.Context) {}) {
		t.Fatal("closing app must reject new lifecycle workers")
	}
}

func TestReservedChatJobCanRegisterDuringShutdown(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.reserveChatJob(); err != nil {
		t.Fatal(err)
	}
	app.beginShutdown()
	jobCtx, cancel := context.WithCancel(app.lifecycleCtx)
	app.registerChatJobCancel("reserved-during-shutdown", cancel)
	workerDone := make(chan struct{})
	go func() {
		defer app.backgroundWG.Done()
		defer close(workerDone)
		<-jobCtx.Done()
		app.unregisterChatJobCancel("reserved-during-shutdown")
	}()
	if err := app.closeResources(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("reserved job was not allowed to finish during shutdown")
	}
}

func TestHTTPBaseContextFollowsAppLifecycle(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	requestCtx := app.server.BaseContext(nil)
	app.beginShutdown()
	select {
	case <-requestCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP base context was not canceled during shutdown")
	}
	if err := app.closeResources(); err != nil {
		t.Fatal(err)
	}
}

func TestAppCloseStopsHTTPServerBeforeClosingStore(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- app.server.Serve(listener) }()
	response, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("unexpected Serve result: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not stop HTTP server")
	}
	client := &http.Client{Timeout: 200 * time.Millisecond}
	if response, err := client.Get("http://" + listener.Addr().String() + "/"); err == nil {
		_ = response.Body.Close()
		t.Fatal("HTTP server still accepted requests after Close")
	}
	if err := app.Close(); err != nil {
		t.Fatalf("second Close must remain idempotent: %v", err)
	}
}

func TestAppShutdownRespectsDeadlineForNonCooperativeWorker(t *testing.T) {
	app, err := NewApp(model.ServerConfig{Addr: "127.0.0.1:0", DataDir: t.TempDir(), WebDir: "../../web"})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	workerDone := make(chan struct{})
	if !app.startBackgroundWork(func(context.Context) {
		<-release
		close(workerDone)
	}) {
		t.Fatal("worker was not started")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	err = app.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown deadline, got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("Shutdown ignored its deadline: %s", elapsed)
	}
	if _, err := app.store.ListWorkspaceSummaries("default"); err != nil {
		t.Fatalf("Store was closed while worker was still running: %v", err)
	}

	close(release)
	select {
	case <-workerDone:
	case <-time.After(time.Second):
		t.Fatal("worker did not finish after release")
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
}
