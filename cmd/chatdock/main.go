package main

import (
	"log"
	"os"

	"chatdock/internal/chatdock"
)

func main() {
	cfg := chatdock.ServerConfig{
		Addr:    getenv("CHATDOCK_ADDR", ":8720"),
		DataDir: getenv("CHATDOCK_DATA", "data"),
		WebDir:  getenv("CHATDOCK_WEB", "web"),
	}

	app, err := chatdock.NewApp(cfg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("ChatDock listening on %s", cfg.Addr)
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
