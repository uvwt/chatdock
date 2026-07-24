package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"chatdock/internal/app"
	"chatdock/internal/model"
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

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(signalCtx, cfg); err != nil {
		log.Fatal(err)
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
