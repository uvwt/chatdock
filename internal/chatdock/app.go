package chatdock

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type App struct {
	cfg       ServerConfig
	store     *Store
	client    *ChatClient
	mcpClient *MCPClient
	server    *http.Server
	runningMu sync.Mutex
	running   map[string]bool
}

func NewApp(cfg ServerConfig) (*App, error) {
	store, err := NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	app := &App{
		cfg:       cfg,
		store:     store,
		client:    NewChatClient(),
		mcpClient: NewMCPClient(),
		running:   make(map[string]bool),
	}
	app.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return app, nil
}

func (a *App) ListenAndServe() error {
	listener, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s failed: %w", a.cfg.Addr, err)
	}

	log.Printf("ChatDock listening on %s", displayListenURL(a.cfg.Addr))
	log.Printf("ChatDock data dir: %s", a.cfg.DataDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.runScheduler(ctx)
	return a.server.Serve(listener)
}

func (a *App) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runDueScheduledTasks()
		}
	}
}

func (a *App) runDueScheduledTasks() {
	tasks, err := a.store.DueScheduledTasksAllPrompts(time.Now())
	if err != nil {
		log.Printf("scheduled task scan failed: %v", err)
		return
	}
	for _, task := range tasks {
		a.startScheduledTask(task.PromptName, task.Task.ID, false)
	}
}

func (a *App) startScheduledTask(promptName string, id string, manual bool) {
	key := strings.TrimSpace(promptName) + ":" + strings.TrimSpace(id)
	if key == "" {
		return
	}
	a.runningMu.Lock()
	if a.running[key] {
		a.runningMu.Unlock()
		return
	}
	a.running[key] = true
	a.runningMu.Unlock()

	go func() {
		defer func() {
			a.runningMu.Lock()
			delete(a.running, key)
			a.runningMu.Unlock()
		}()
		if _, err := a.executeScheduledTask(context.Background(), promptName, id, manual); err != nil {
			log.Printf("scheduled task %s failed: %v", key, err)
		}
	}()
}

func (a *App) executeScheduledTask(ctx context.Context, promptName string, id string, manual bool) (ScheduledTaskRunResponse, error) {
	startedAt := time.Now()
	run, err := a.store.PrepareScheduledTaskRunInPrompt(promptName, id, manual, startedAt)
	if err != nil {
		return ScheduledTaskRunResponse{}, err
	}
	answer, runErr := a.completeWithOptionalTools(ctx, run.Config, run.History)
	result, finishErr := a.store.FinishScheduledTaskRun(run.PromptName, run.Task.ID, run.SessionID, answer, startedAt, manual, runErr)
	if finishErr != nil {
		return ScheduledTaskRunResponse{}, finishErr
	}
	if runErr != nil {
		return result, runErr
	}
	return result, nil
}

func displayListenURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "0.0.0.0:") {
		return "http://127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	return "http://" + addr
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/config", a.handleGetConfig)
	mux.HandleFunc("POST /api/config", a.handleSaveConfig)
	mux.HandleFunc("GET /api/prompts", a.handleListPrompts)
	mux.HandleFunc("POST /api/prompts", a.handleCreatePrompt)
	mux.HandleFunc("POST /api/prompts/select", a.handleSelectPrompt)
	mux.HandleFunc("GET /api/mcp-config", a.handleGetMCPConfig)
	mux.HandleFunc("POST /api/mcp-config", a.handleSaveMCPConfig)
	mux.HandleFunc("GET /api/mcp/tools", a.handleListMCPTools)
	mux.HandleFunc("GET /api/mcp/test", a.handleTestMCPServer)
	mux.HandleFunc("POST /api/mcp/call", a.handleCallMCPTool)
	mux.HandleFunc("GET /api/skills", a.handleListSkills)
	mux.HandleFunc("POST /api/skills", a.handleCreateSkill)
	mux.HandleFunc("/api/skills/", a.handleSkillRoute)
	mux.HandleFunc("GET /api/scheduled-tasks", a.handleListScheduledTasks)
	mux.HandleFunc("POST /api/scheduled-tasks", a.handleCreateScheduledTask)
	mux.HandleFunc("/api/scheduled-tasks/", a.handleScheduledTaskRoute)
	mux.HandleFunc("GET /api/sessions", a.handleListSessions)
	mux.HandleFunc("POST /api/sessions", a.handleCreateSession)
	mux.HandleFunc("/api/sessions/", a.handleSessionRoute)
	mux.HandleFunc("POST /api/chat", a.handleChat)
	mux.HandleFunc("POST /api/chat/stream", a.handleChatStream)

	fileServer := http.FileServer(http.Dir(a.cfg.WebDir))
	mux.Handle("/", fileServer)

	return logRequest(a.authMiddleware(mux))
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (a *App) authMiddleware(next http.Handler) http.Handler {
	token := strings.TrimSpace(a.cfg.AuthToken)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/app.") || strings.HasPrefix(r.URL.Path, "/markdown.js") || strings.HasPrefix(r.URL.Path, "/mcp.js") {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if got == "" {
			got = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		if got != token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RemoveLegacyRuntimeArtifacts(repoDir string) error {
	for _, name := range []string{"chatdock", "bin"} {
		path := repoDir + string(os.PathSeparator) + name
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
