package config

import (
	"path/filepath"
	"testing"
)

func TestLoadReadsLoggingConfig(t *testing.T) {
	t.Setenv("LOG_ENABLE_FILE", "true")
	t.Setenv("LOG_DIR", filepath.Join("runtime", "logs"))
	t.Setenv("LOG_FILE_NAME", "admin-api.log")
	t.Setenv("LOG_MAX_TAIL_LINES", "1000")
	t.Setenv("LOG_ALLOWED_EXTENSIONS", ".log,.jsonl")
	t.Setenv("LOG_FILE_MAX_SIZE_MB", "64")
	t.Setenv("LOG_FILE_MAX_BACKUPS", "7")
	t.Setenv("LOG_FILE_MAX_AGE_DAYS", "14")
	t.Setenv("LOG_FILE_COMPRESS", "true")

	cfg := Load()

	if !cfg.Logging.EnableFile {
		t.Fatalf("expected file logging enabled")
	}
	if cfg.Logging.Dir != filepath.Join("runtime", "logs") || cfg.Logging.FileName != "admin-api.log" {
		t.Fatalf("unexpected logging path config: %#v", cfg.Logging)
	}
	if cfg.Logging.MaxTailLines != 1000 {
		t.Fatalf("expected max tail lines 1000, got %d", cfg.Logging.MaxTailLines)
	}
	if len(cfg.Logging.AllowedExtensions) != 2 || cfg.Logging.AllowedExtensions[0] != ".log" || cfg.Logging.AllowedExtensions[1] != ".jsonl" {
		t.Fatalf("unexpected extensions: %#v", cfg.Logging.AllowedExtensions)
	}
	if cfg.Logging.FileMaxSizeMB != 64 || cfg.Logging.FileMaxBackups != 7 || cfg.Logging.FileMaxAgeDays != 14 || !cfg.Logging.FileCompress {
		t.Fatalf("unexpected file rotation config: %#v", cfg.Logging)
	}
}
