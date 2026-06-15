// The api command is the composition root: it loads config, builds the
// adapters, wires them through ports by plain constructor injection, and
// owns the process lifecycle (design D7).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpadapter "github.com/ramdanaguss/selaras/server/internal/adapter/http"
	"github.com/ramdanaguss/selaras/server/internal/adapter/postgres"
	"github.com/ramdanaguss/selaras/server/internal/config"
)

const shutdownDrain = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(configuration.Env)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	pool, err := postgres.NewPool(ctx, configuration.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	router := httpadapter.NewRouter(httpadapter.RouterConfig{
		Logger:     logger,
		Pinger:     postgres.NewPinger(pool),
		CORSOrigin: configuration.CORSOrigin,
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", configuration.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()
	logger.Info("api listening", "port", configuration.Port, "env", configuration.Env)

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutdown signal received, draining", "timeout", shutdownDrain.String())

	drainCtx, cancel := context.WithTimeout(context.Background(), shutdownDrain)
	defer cancel()

	if err := server.Shutdown(drainCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("draining http server: %w", err)
	}

	logger.Info("shutdown complete")

	return nil
}

func newLogger(environment string) *slog.Logger {
	if environment == config.EnvProduction {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
