package app

import (
	"context"
	"errors"
	"testing"

	"chatdock/internal/model"
)

func TestRunRejectsCancelledContextBeforeCreatingServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Run(ctx, model.ServerConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}
