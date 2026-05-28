package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"admin_back_go/internal/config"

	"gopkg.in/natefinch/lumberjack.v2"
)

func NewLogger(cfg config.LoggingConfig, stdout io.Writer) (*slog.Logger, io.Closer, error) {
	if stdout == nil {
		stdout = os.Stdout
	}
	writer := stdout
	var closer io.Closer
	if cfg.EnableFile {
		if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
			return nil, nil, err
		}
		fileWriter := &lumberjack.Logger{
			Filename:   filepath.Join(cfg.Dir, cfg.FileName),
			MaxSize:    cfg.FileMaxSizeMB,
			MaxBackups: cfg.FileMaxBackups,
			MaxAge:     cfg.FileMaxAgeDays,
			Compress:   cfg.FileCompress,
		}
		writer = io.MultiWriter(stdout, fileWriter)
		closer = fileWriter
	}
	return slog.New(slog.NewJSONHandler(writer, nil)), closer, nil
}
