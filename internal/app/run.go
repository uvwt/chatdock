package app

import (
	"context"
	"errors"
	"time"

	"chatdock/internal/httpapi"
	"chatdock/internal/model"
)

// Run 创建 HTTP 服务并在进程上下文结束时完成有界关闭。
func Run(ctx context.Context, cfg model.ServerConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	server, err := httpapi.NewServer(cfg)
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	shutdown := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}

	select {
	case err := <-serveErr:
		return errors.Join(err, shutdown())
	case <-ctx.Done():
		return errors.Join(shutdown(), <-serveErr)
	}
}
