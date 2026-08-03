package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	contextengine "admin_back_go/internal/module/ai/contextengine"
)

func TestCutoverPreflightRejectsMissingDSNWithoutOutput(t *testing.T) {
	var output strings.Builder
	err := run(context.Background(), nil, commandDependencies{getenv: func(string) string { return "" }, open: func(config.MySQLConfig) (*database.Client, error) { t.Fatal("database opened"); return nil, nil }, check: contextengine.RunCutoverPreflight, stdout: &output})
	if err == nil || output.Len() != 0 || strings.Contains(err.Error(), "password") {
		t.Fatalf("err=%v output=%q", err, output.String())
	}
}

func TestCutoverPreflightViolationErrorIsFixed(t *testing.T) {
	if !errors.Is(errCutoverViolations, errCutoverViolations) || strings.Contains(errCutoverViolations.Error(), "query") {
		t.Fatalf("unsafe error=%v", errCutoverViolations)
	}
}
