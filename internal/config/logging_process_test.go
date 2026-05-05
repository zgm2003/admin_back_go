package config

import "testing"

func TestLoggingConfigForProcessUsesDedicatedWorkerFile(t *testing.T) {
	t.Setenv("LOG_FILE_NAME", "admin-api.log")
	t.Setenv("LOG_WORKER_FILE_NAME", "admin-worker.log")

	cfg := Load()
	workerLogging := cfg.Logging.ForProcess("admin-worker")

	if workerLogging.FileName != "admin-worker.log" {
		t.Fatalf("expected admin-worker to write admin-worker.log, got %q", workerLogging.FileName)
	}
	if cfg.Logging.FileName != "admin-api.log" {
		t.Fatalf("ForProcess must not mutate base config, got %q", cfg.Logging.FileName)
	}
}

func TestLoggingConfigForProcessKeepsAPIFile(t *testing.T) {
	t.Setenv("LOG_FILE_NAME", "admin-api.log")
	t.Setenv("LOG_WORKER_FILE_NAME", "admin-worker.log")

	cfg := Load()
	apiLogging := cfg.Logging.ForProcess("admin-api")

	if apiLogging.FileName != "admin-api.log" {
		t.Fatalf("expected admin-api to write admin-api.log, got %q", apiLogging.FileName)
	}
}
