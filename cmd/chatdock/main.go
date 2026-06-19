package main

import (
	"log"
	"os"

	"chatdock/internal/chatdock"
)

func main() {
	cfg := chatdock.ServerConfig{
		Addr:      getenv("CHATDOCK_ADDR", ":8720"),
		DataDir:   getenv("CHATDOCK_DATA", defaultDataDir()),
		WebDir:    os.Getenv("CHATDOCK_WEB"),
		AuthToken: os.Getenv("CHATDOCK_AUTH_TOKEN"),
	}

	app, err := chatdock.NewApp(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := app.ListenAndServe(); err != nil {
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
