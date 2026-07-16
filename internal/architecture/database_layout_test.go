package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseLayoutSeparatesLegacyAndAtlasMigrations(t *testing.T) {
	root := backendRoot(t)
	legacy, err := filepath.Glob(filepath.Join(root, "database", "legacy-migrations", "*.sql"))
	if err != nil {
		t.Fatalf("glob legacy migrations: %v", err)
	}
	if len(legacy) < 40 {
		t.Fatalf("legacy migrations=%d", len(legacy))
	}

	active, err := filepath.Glob(filepath.Join(root, "database", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("glob active migrations: %v", err)
	}
	if len(active) != 1 || filepath.Base(active[0]) != "202607150001_baseline.sql" {
		t.Fatalf("active migrations=%v", active)
	}

	data, err := os.ReadFile(filepath.Join(root, "scripts", "database", "atlas.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a") {
		t.Fatal("Atlas image is not digest pinned")
	}
}
