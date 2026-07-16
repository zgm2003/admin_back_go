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

	atlasSum, err := os.ReadFile(filepath.Join(root, "database", "migrations", "atlas.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(atlasSum), "h1:") || !strings.Contains(string(atlasSum), "202607150001_baseline.sql h1:") {
		t.Fatalf("Atlas checksum does not cover the baseline migration: %s", atlasSum)
	}
	if _, err := os.Stat(filepath.Join(root, "atlas.sum")); !os.IsNotExist(err) {
		t.Fatal("Atlas checksum must have a single source in database/migrations")
	}

	attributes, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		"/database/migrations/*.sql text eol=lf",
		"/database/migrations/atlas.sum text eol=lf",
	} {
		if !strings.Contains(string(attributes), rule) {
			t.Fatalf("missing LF guard %q", rule)
		}
	}
}
