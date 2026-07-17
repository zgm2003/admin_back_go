package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/logging"
	runtimepkg "admin_back_go/internal/runtime"
)

const shutdownTimeout = 15 * time.Second

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

	process, err := runtimepkg.NewWorker(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize admin worker", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := process.Start(ctx); err != nil {
		logger.Error("process failed", "process", string(config.ProcessWorker), "error", err)
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := process.Shutdown(shutdownCtx); err != nil {
		logger.Error("process shutdown failed", "process", string(config.ProcessWorker), "error", err)
		os.Exit(1)
	}
	logger.Info("process stopped", "process", string(config.ProcessWorker))
}
