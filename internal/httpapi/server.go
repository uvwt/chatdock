package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatdock/internal/agentdock"
	"chatdock/internal/llm"
	"chatdock/internal/mcp"
	"chatdock/internal/model"
	storepkg "chatdock/internal/store"
	"chatdock/internal/toolapproval"
)

type Server struct {
	cfg             model.ServerConfig
	store           *storepkg.Store
	client          *llm.ChatClient
	mcpClient       *mcp.MCPClient
	agentDock       *agentdock.Client
	server          *http.Server
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	jobMu           sync.Mutex
	jobCancel       map[string]context.CancelFunc
	jobGuidance     map[string][]chatJobGuidance
	backgroundWG    sync.WaitGroup
	closing         bool
	shutdownOnce    sync.Once
	closeOnce       sync.Once
	closeErr        error
	approvals       *toolapproval.Service
	runningMu       sync.Mutex
	running         map[string]bool
	embeddingMu     sync.Mutex
	embeddingMemo   map[string][]float64
}

func normalizeServerConfig(cfg model.ServerConfig) model.ServerConfig {
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	return cfg
}

func NewServer(cfg model.ServerConfig) (*Server, error) {
	cfg = normalizeServerConfig(cfg)
	st, err := storepkg.NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	server := &Server{
		cfg:             cfg,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
		store:           st,
		client:          llm.NewChatClient(),
		mcpClient:       mcp.NewMCPClient(),
		agentDock:       agentdock.NewClient(cfg.AgentDockContextURL, cfg.AgentDockContextToken),
		jobCancel:       make(map[string]context.CancelFunc),
		jobGuidance:     make(map[string][]chatJobGuidance),
		approvals:       toolapproval.NewService(st),
		running:         make(map[string]bool),
		embeddingMemo:   make(map[string][]float64),
	}
	server.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return lifecycleCtx
		},
	}
	return server, nil
}

func (a *Server) ListenAndServe() error {
	listener, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s failed: %w", a.cfg.Addr, err)
	}
	log.Printf("ChatDock listening on %s", displayListenURL(a.cfg.Addr))
	log.Printf("ChatDock data dir: %s", a.cfg.DataDir)
	a.startBackgroundWork(a.runScheduler)
	a.startBackgroundWork(a.runChatJobEventCleanup)
	a.startBackgroundWork(a.warmToolEmbeddingIndex)
	if err := a.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (a *Server) startBackgroundWork(run func(context.Context)) bool {
	if run == nil {
		return false
	}
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	if a.closing {
		return false
	}
	a.backgroundWG.Add(1)
	go func() {
		defer a.backgroundWG.Done()
		run(a.lifecycleCtx)
	}()
	return true
}

func (a *Server) Shutdown(ctx context.Context) error {
	a.beginShutdown()
	shutdownErr := a.server.Shutdown(ctx)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, a.server.Close())
	}
	if waitErr := a.waitForBackground(ctx); waitErr != nil {
		return errors.Join(shutdownErr, waitErr)
	}
	return errors.Join(shutdownErr, a.closeResources())
}

func (a *Server) Close() error {
	a.beginShutdown()
	serverErr := a.server.Close()
	if errors.Is(serverErr, http.ErrServerClosed) || errors.Is(serverErr, net.ErrClosed) {
		serverErr = nil
	}
	return errors.Join(serverErr, a.closeResources())
}

func (a *Server) beginShutdown() {
	a.shutdownOnce.Do(func() {
		a.jobMu.Lock()
		a.closing = true
		cancels := make([]context.CancelFunc, 0, len(a.jobCancel))
		for _, cancel := range a.jobCancel {
			cancels = append(cancels, cancel)
		}
		a.jobMu.Unlock()

		if a.lifecycleCancel != nil {
			a.lifecycleCancel()
		}
		for _, cancel := range cancels {
			cancel()
		}
	})
}

func (a *Server) waitForBackground(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		a.backgroundWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Server) closeResources() error {
	a.backgroundWG.Wait()
	var mcpErr error
	if a.mcpClient != nil {
		mcpErr = a.mcpClient.Close()
	}
	return errors.Join(mcpErr, a.closeStore())
}

func (a *Server) closeStore() error {
	a.closeOnce.Do(func() {
		if a.store != nil {
			a.closeErr = a.store.Close()
		}
	})
	return a.closeErr
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
