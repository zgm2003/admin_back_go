package main

import (
	"log/slog"
	"os"

	"admin_back_go/internal/bootstrap"
	"admin_back_go/internal/config"
)

func main() {
	_ = config.LoadDotEnv()
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	app := bootstrap.New(cfg, logger)
	if err := app.Run(); err != nil {
		logger.Error("admin api stopped", "error", err)
		os.Exit(1)
	}
}
