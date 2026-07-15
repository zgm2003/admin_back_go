package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"admin_back_go/internal/bootstrap"
	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/logging"
	"admin_back_go/internal/infra/taskqueue"
)

func main() {
	_ = config.LoadDotEnv()
	cfg, err := config.Load(config.ProcessWorker)
	if err != nil {
		slog.Error("invalid environment configuration", "error", err)
		os.Exit(1)
	}
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), taskqueue.DefaultShutdownTimeout)
	defer cancel()

	if err := worker.Shutdown(shutdownCtx); err != nil {
		logger.Error("admin worker shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("admin worker stopped")
}
