package main

import (
	"log/slog"
	"os"

	"admin_back_go/internal/bootstrap"
	"admin_back_go/internal/config"
	"admin_back_go/internal/platform/logging"
)

func main() {
	_ = config.LoadDotEnv()
	cfg := config.Load()
	logger, logCloser, err := logging.NewLogger(cfg.Logging, os.Stdout)
	if err != nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
		logger.Error("failed to initialize logger", "error", err)
	} else if logCloser != nil {
		defer logCloser.Close()
	}

	app := bootstrap.New(cfg, logger)
	if err := app.Run(); err != nil {
		logger.Error("admin api stopped", "error", err)
		os.Exit(1)
	}
}
