package chatdock

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type App struct {
	cfg    ServerConfig
	store  *Store
	client *ChatClient
	server *http.Server
}

func NewApp(cfg ServerConfig) (*App, error) {
	store, err := NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	app := &App{
		cfg:    cfg,
		store:  store,
		client: NewChatClient(),
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
	return a.server.Serve(listener)
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
	mux.HandleFunc("GET /api/sessions", a.handleListSessions)
	mux.HandleFunc("POST /api/sessions", a.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}", a.handleGetSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", a.handleDeleteSession)
	mux.HandleFunc("POST /api/chat", a.handleChat)
	mux.HandleFunc("POST /api/chat/stream", a.handleChatStream)

	fileServer := http.FileServer(http.Dir(a.cfg.WebDir))
	mux.Handle("/", fileServer)

	return logRequest(mux)
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
