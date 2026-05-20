package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadReadsOnlyLogDirFromEnvironment(t *testing.T) {
	t.Setenv("LOG_DIR", filepath.Join("custom", "logs"))
	t.Setenv("LOG_ENABLE_FILE", "false")
	t.Setenv("LOG_FILE_NAME", "legacy.log")
	t.Setenv("LOG_API_FILE_NAME", "api-custom.log")
	t.Setenv("LOG_WORKER_FILE_NAME", "worker-custom.log")
	t.Setenv("LOG_MAX_TAIL_LINES", "1000")
	t.Setenv("LOG_ALLOWED_EXTENSIONS", ".log,.jsonl")
	t.Setenv("LOG_FILE_MAX_SIZE_MB", "1")
	t.Setenv("LOG_FILE_MAX_BACKUPS", "1")
	t.Setenv("LOG_FILE_MAX_AGE_DAYS", "1")
	t.Setenv("LOG_FILE_COMPRESS", "false")

	cfg := Load()

	if !cfg.Logging.EnableFile {
		t.Fatalf("LOG_ENABLE_FILE must be code-owned and default true")
	}
	if cfg.Logging.Dir != filepath.Join("custom", "logs") {
		t.Fatalf("expected LOG_DIR override to be used, got %q", cfg.Logging.Dir)
	}
	if cfg.Logging.FileName != "admin-api.log" || cfg.Logging.APIFileName != "admin-api.log" {
		t.Fatalf("api log file names must be code-owned defaults, got %#v", cfg.Logging)
	}
	if cfg.Logging.WorkerFileName != "admin-worker.log" {
		t.Fatalf("worker log file name must be code-owned default, got %#v", cfg.Logging)
	}
	if cfg.Logging.MaxTailLines != 2000 {
		t.Fatalf("max tail lines must be code-owned default 2000, got %d", cfg.Logging.MaxTailLines)
	}
	if !reflect.DeepEqual(cfg.Logging.AllowedExtensions, []string{".log"}) {
		t.Fatalf("allowed extensions must be code-owned .log only, got %#v", cfg.Logging.AllowedExtensions)
	}
	if cfg.Logging.FileMaxSizeMB != 64 || cfg.Logging.FileMaxBackups != 7 || cfg.Logging.FileMaxAgeDays != 14 || !cfg.Logging.FileCompress {
		t.Fatalf("file rotation config must be code-owned defaults, got %#v", cfg.Logging)
	}
}
