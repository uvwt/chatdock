package chatdock

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	storepkg "chatdock/internal/chatdock/store"
)

type App struct {
	cfg                   model.ServerConfig
	store                 *storepkg.Store
	client                *llm.ChatClient
	mcpClient             *mcp.MCPClient
	server                *http.Server
	jobMu                 sync.Mutex
	jobCancel             map[string]context.CancelFunc
	confirmMu             sync.Mutex
	confirmations         map[string]*MCPConfirmation
	runningMu             sync.Mutex
	running               map[string]bool
	embeddingMu           sync.Mutex
	embeddingMemo         map[string][]float64
	agentDockContextMu    sync.Mutex
	agentDockContext      string
	agentDockContextUntil time.Time
}

func NewApp(cfg model.ServerConfig) (*App, error) {
	st, err := storepkg.NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	app := &App{
		cfg:           cfg,
		store:         st,
		client:        llm.NewChatClient(),
		mcpClient:     mcp.NewMCPClient(),
		jobCancel:     make(map[string]context.CancelFunc),
		confirmations: make(map[string]*MCPConfirmation),
		running:       make(map[string]bool),
		embeddingMemo: make(map[string][]float64),
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
	go a.warmToolEmbeddingIndex(ctx)
	return a.server.Serve(listener)
}

func (a *App) Close() error {
	if a.store == nil {
		return nil
	}
	return a.store.Close()
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
