package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"admin_back_go/internal/bootstrap"
	"admin_back_go/internal/config"
	"admin_back_go/internal/platform/logging"
)

func main() {
	_ = config.LoadDotEnv()
	cfg := config.Load()
	logger, logCloser, err := logging.NewLogger(cfg.Logging.ForProcess("admin-worker"), os.Stdout)
	if err != nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
		logger.Error("failed to initialize logger", "error", err)
	} else if logCloser != nil {
		defer logCloser.Close()
	}

	worker, err := bootstrap.NewWorker(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize admin worker", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := worker.Start(ctx); err != nil {
		logger.Error("admin worker start failed", "error", err)
		_ = worker.Shutdown(context.Background())
		os.Exit(1)
	}

	<-ctx.Done()
	shutdownTimeout := cfg.Queue.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10 * time.Second
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := worker.Shutdown(shutdownCtx); err != nil {
		logger.Error("admin worker shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("admin worker stopped")
}
