package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chatdock/internal/chatdock"
	"chatdock/internal/chatdock/model"
)

func main() {
	cfg := model.ServerConfig{
		Addr:                  getenv("CHATDOCK_ADDR", ":8720"),
		DataDir:               getenv("CHATDOCK_DATA", defaultDataDir()),
		WebDir:                os.Getenv("CHATDOCK_WEB"),
		AuthToken:             os.Getenv("CHATDOCK_AUTH_TOKEN"),
		AuthUsername:          os.Getenv("CHATDOCK_AUTH_USERNAME"),
		AuthCredential:        os.Getenv("CHATDOCK_AUTH_CREDENTIAL"),
		PublicBaseURL:         os.Getenv("CHATDOCK_PUBLIC_BASE_URL"),
		EmbeddingBaseURL:      os.Getenv("CHATDOCK_EMBEDDING_BASE_URL"),
		EmbeddingAPIKey:       os.Getenv("CHATDOCK_EMBEDDING_API_KEY"),
		EmbeddingModel:        getenv("CHATDOCK_EMBEDDING_MODEL", "BAAI/bge-m3"),
		AgentDockContextURL:   os.Getenv("CHATDOCK_AGENTDOCK_CONTEXT_URL"),
		AgentDockContextToken: os.Getenv("CHATDOCK_AGENTDOCK_CONTEXT_TOKEN"),
	}

	app, err := chatdock.NewApp(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := run(app); err != nil {
		log.Fatal(err)
	}
}

func run(app *chatdock.App) error {
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- app.ListenAndServe()
	}()

	shutdown := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return app.Shutdown(shutdownCtx)
	}

	select {
	case err := <-serveErr:
		return errors.Join(err, shutdown())
	case <-signalCtx.Done():
		return errors.Join(shutdown(), <-serveErr)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return dir + "/chatdock"
	}
	return "data"
}
