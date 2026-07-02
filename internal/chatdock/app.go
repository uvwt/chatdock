package chatdock

import (
	"bytes"
	"context"
	"crypto/subtle"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	webui "chatdock/web"
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
	defer func() { _ = a.Close() }()

	log.Printf("ChatDock listening on %s", displayListenURL(a.cfg.Addr))
	log.Printf("ChatDock data dir: %s", a.cfg.DataDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.runScheduler(ctx)
	return a.server.Serve(listener)
}

func (a *App) Close() error {
	if a.store == nil {
		return nil
	}
	return a.store.Close()
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
	answer, runErr := a.completeWithOptionalTools(ctx, run.SessionID, run.Config, run.History)
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
	mux.HandleFunc("GET /api/auth/status", a.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/login", a.handleAuthLogin)
	mux.HandleFunc("GET /api/setup/status", a.handleSetupStatus)
	mux.HandleFunc("POST /api/setup/init", a.handleSetupInit)
	mux.HandleFunc("GET /api/model-providers", a.handleListModelProviders)
	mux.HandleFunc("POST /api/model-providers/test", a.handleTestModelProvider)
	mux.HandleFunc("POST /api/model-providers/models", a.handleListProviderModels)
	mux.HandleFunc("GET /api/workspaces", a.handleListWorkspaces)
	mux.HandleFunc("POST /api/workspaces", a.handleCreateWorkspace)
	mux.HandleFunc("/api/workspaces/", a.handleWorkspaceRoute)
	mux.HandleFunc("GET /api/data/status", a.handleDataStatus)
	mux.HandleFunc("GET /api/system/status", a.handleSystemStatus)
	mux.HandleFunc("GET /api/config", a.handleGetConfig)
	mux.HandleFunc("POST /api/config", a.handleSaveConfig)
	mux.HandleFunc("GET /api/prompts", a.handleListPrompts)
	mux.HandleFunc("POST /api/prompts", a.handleCreatePrompt)
	mux.HandleFunc("POST /api/prompts/select", a.handleSelectPrompt)
	mux.HandleFunc("GET /api/mcp-config", a.handleGetMCPConfig)
	mux.HandleFunc("POST /api/mcp-config", a.handleSaveMCPConfig)
	mux.HandleFunc("GET /api/mcp/tools", a.handleListMCPTools)
	mux.HandleFunc("GET /api/mcp/status", a.handleMCPStatus)
	mux.HandleFunc("GET /api/mcp/test", a.handleTestMCPServer)
	mux.HandleFunc("POST /api/mcp/call", a.handleCallMCPTool)
	mux.HandleFunc("GET /api/runs", a.handleListRuns)
	mux.HandleFunc("GET /api/runs/{id}", a.handleRunDetail)
	mux.HandleFunc("GET /api/agent-tasks", a.handleListAgentTasks)
	mux.HandleFunc("GET /api/skills", a.handleListSkills)
	mux.HandleFunc("POST /api/skills", a.handleCreateSkill)
	mux.HandleFunc("/api/skills/", a.handleSkillRoute)
	mux.HandleFunc("GET /api/scheduled-tasks", a.handleListScheduledTasks)
	mux.HandleFunc("POST /api/scheduled-tasks", a.handleCreateScheduledTask)
	mux.HandleFunc("/api/scheduled-tasks/", a.handleScheduledTaskRoute)
	mux.HandleFunc("GET /api/sessions", a.handleListSessions)
	mux.HandleFunc("POST /api/sessions", a.handleCreateSession)
	mux.HandleFunc("/api/sessions/", a.handleSessionRoute)
	mux.HandleFunc("POST /api/files", a.handleUploadFile)
	mux.HandleFunc("POST /api/chat", a.handleChat)
	mux.HandleFunc("POST /api/chat/stream", a.handleChatStream)
	mux.HandleFunc("GET /api/chat/jobs", a.handleListChatJobs)
	mux.HandleFunc("POST /api/chat/jobs", a.handleCreateChatJob)
	mux.HandleFunc("GET /api/chat/jobs/{id}/events", a.handleChatJobEvents)

	mux.Handle("/", a.webHandler())

	return logRequest(a.authMiddleware(mux))
}

func (a *App) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(a.cfg.AuthUsername)
	writeJSONResponse(w, http.StatusOK, AuthStatusResponse{
		Enabled:      strings.TrimSpace(a.cfg.AuthToken) != "",
		LoginEnabled: username != "" && strings.TrimSpace(a.cfg.AuthCredential) != "",
		Username:     username,
	})
}

func (a *App) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var input AuthLoginRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	token := strings.TrimSpace(a.cfg.AuthToken)
	if token == "" {
		writeJSONResponse(w, http.StatusOK, AuthLoginResponse{OK: true})
		return
	}

	username := strings.TrimSpace(a.cfg.AuthUsername)
	credential := strings.TrimSpace(a.cfg.AuthCredential)
	if username == "" || credential == "" {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("login is not configured"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(input.Username), []byte(username)) == 1 && subtle.ConstantTimeCompare([]byte(input.Credential), []byte(credential)) == 1 {
		writeJSONResponse(w, http.StatusOK, AuthLoginResponse{OK: true, Token: token, Username: username})
		return
	}
	writeError(w, http.StatusUnauthorized, fmt.Errorf("invalid login"))
}

func (a *App) webHandler() http.Handler {
	var webFS fs.FS
	if dir := strings.TrimSpace(a.cfg.WebDir); dir != "" {
		webFS = os.DirFS(dir)
	} else {
		embedded, err := fs.Sub(webui.Dist, "dist")
		if err != nil {
			log.Printf("embedded web dist unavailable: %v", err)
			return http.NotFoundHandler()
		}
		webFS = embedded
	}
	return spaFileServer(webFS)
}

func spaFileServer(webFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 前端路由只接管页面路径；API/MCP 路由缺失时必须保持后端 404，不能回落成 index.html。
		if isBackendRoute(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if file, err := webFS.Open(name); err == nil {
			stat, statErr := file.Stat()
			_ = file.Close()
			if statErr == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}

		index, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
}

func isBackendRoute(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") || requestPath == "/mcp" || strings.HasPrefix(requestPath, "/mcp/")
}

func isPublicBackendRoute(requestPath string) bool {
	return requestPath == "/api/auth/status" || requestPath == "/api/auth/login"
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
		if !isBackendRoute(r.URL.Path) || isPublicBackendRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
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
