package architecture

import (
	"os"
	"path/filepath"
	"regexp"
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

func TestDatabaseVerificationWorkflowPinsImmutableInputs(t *testing.T) {
	root := backendRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "verify-database.yml")
	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read database verification workflow: %v", err)
	}
	scriptPath := filepath.Join(root, "scripts", "verify-database.ps1")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read shared database verifier: %v", err)
	}
	expandedVerifier, err := os.ReadFile(filepath.Join(root, "scripts", "database", "verify-expanded-schema.ps1"))
	if err != nil {
		t.Fatalf("read expanded-schema verifier: %v", err)
	}

	workflowText := string(workflow)
	combined := workflowText + "\n" + string(script)
	verificationText := string(script) + "\n" + string(expandedVerifier)
	for _, required := range []string{
		"mysql:8.4.10",
		"arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a",
		"scripts/verify-database.ps1",
		"empty:",
		"imported:",
		"actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("database verification is missing immutable input %q", required)
		}
	}
	if regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*\S+@v[0-9]`).Match(workflow) {
		t.Fatal("database workflow contains a mutable action tag")
	}

	for _, required := range []string{
		"all-nondestructive",
		"030_verify_schema.sql",
		"036_verify_export_cleanup.sql",
		"query-manifest",
		"git ls-files",
		"admin-go.env",
		".cnf",
		"schema_sha256",
		"--sql-mode=NO_ENGINE_SUBSTITUTION",
		"schema convergence diagnostic",
	} {
		if !strings.Contains(verificationText, required) {
			t.Fatalf("shared database verifier is missing gate %q", required)
		}
	}
	if !strings.Contains(string(script), `${Database}?parseTime`) {
		t.Fatal("database verifier must delimit the schema variable before DSN query parameters")
	}
	if !strings.Contains(string(script), "$primaryError") || strings.Contains(string(script), "-not $?") {
		t.Fatal("database verifier must preserve the primary failure independently from cleanup command status")
	}
}

func TestDatabaseVerifierFixturePreservesCanonicalCheckConstraints(t *testing.T) {
	root := backendRoot(t)
	script, err := os.ReadFile(filepath.Join(root, "scripts", "verify-database.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"ai_reply_commands", "ai_video_tasks", "authz_principal_versions", "schema_reconciliation_runs",
	} {
		pattern := regexp.MustCompile(`(?i)DROP\s+TABLE\s+` + "`" + regexp.QuoteMeta(table) + "`" + `\s*;`)
		if pattern.Match(script) {
			t.Fatalf("synthetic fixture recreates canonical CHECK constraint text for %s", table)
		}
	}
}
