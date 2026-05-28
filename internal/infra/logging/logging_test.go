package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"admin_back_go/internal/config"
)

func TestNewLoggerWritesStdoutAndRotatedFile(t *testing.T) {
	var stdout bytes.Buffer
	logDir := t.TempDir()
	logger, closer, err := NewLogger(config.LoggingConfig{
		EnableFile:        true,
		Dir:               logDir,
		FileName:          "admin-api.log",
		FileMaxSizeMB:     1,
		FileMaxBackups:    1,
		FileMaxAgeDays:    1,
		FileCompress:      false,
		MaxTailLines:      500,
		AllowedExtensions: []string{".log"},
	}, &stdout)
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}
	logger.Info("test log", "level_probe", "ok")
	if closer != nil {
		if err := closer.Close(); err != nil {
			t.Fatalf("close logger: %v", err)
		}
	}

	if !strings.Contains(stdout.String(), "test log") {
		t.Fatalf("expected stdout log, got %s", stdout.String())
	}
	fileBytes, err := os.ReadFile(filepath.Join(logDir, "admin-api.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(fileBytes), "test log") || !strings.Contains(string(fileBytes), "level_probe") {
		t.Fatalf("expected json log in file, got %s", string(fileBytes))
	}
}

func TestNewLoggerStdoutOnly(t *testing.T) {
	var stdout bytes.Buffer
	logger, closer, err := NewLogger(config.LoggingConfig{EnableFile: false}, &stdout)
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}
	if closer != nil {
		t.Fatalf("expected nil closer for stdout-only logger")
	}
	logger.LogAttrs(nil, slog.LevelInfo, "stdout only")
	if !strings.Contains(stdout.String(), "stdout only") {
		t.Fatalf("expected stdout-only log, got %s", stdout.String())
	}
}
