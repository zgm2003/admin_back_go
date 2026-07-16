package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCanonicalDatabaseBaselineIsPinnedAndConvergent(t *testing.T) {
	root := backendRoot(t)
	schema, err := os.ReadFile(filepath.Join(root, "database", "schema", "admin.hcl"))
	if err != nil {
		t.Fatalf("read canonical admin schema: %v", err)
	}
	body := string(schema)
	for _, required := range []string{
		`schema "admin"`, `table "users"`, `table "ai_image_files"`,
		`table "schema_reconciliation_runs"`, `table "atlas_schema_revisions"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("canonical schema missing %q", required)
		}
	}
	if regexp.MustCompile(`(?mi)^\s*auto_increment\s*=\s*[0-9]+\s*$`).Match(schema) {
		t.Fatal("canonical schema contains a volatile auto-increment counter")
	}
	if regexp.MustCompile(`(?i)\bdefiner\b`).Match(schema) {
		t.Fatal("canonical schema contains a definer")
	}

	establish := mustReadBaselineScript(t, root, "establish-baseline.ps1")
	for _, required := range []string{
		"atlas-runtime-common.ps1", "admin:atlas:migrate", "lock-run", "migrate", "set", "202607150001",
		"'migrate', 'validate'", "schema", "inspect", "ExpectedFingerprint", "Move-Item",
	} {
		if !strings.Contains(establish, required) {
			t.Fatalf("establish baseline script missing %q", required)
		}
	}
	runtimeCommon := mustReadBaselineScript(t, root, "atlas-runtime-common.ps1")
	if !strings.Contains(runtimeCommon, "arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a") {
		t.Fatal("Atlas runtime helper is not digest pinned")
	}

	drift := mustReadBaselineScript(t, root, "check-drift.ps1")
	for _, required := range []string{
		"^admin_(empty|imported)_[0-9a-f]{12}$", "schema", "apply", "--auto-approve",
		"admin.hcl", "fingerprint", "schema_sha256",
	} {
		if !strings.Contains(drift, required) {
			t.Fatalf("drift checker missing %q", required)
		}
	}
}

func TestEstablishBaselineValidatesHCLWithoutAtlasDevDatabase(t *testing.T) {
	root := backendRoot(t)
	establish := mustReadBaselineScript(t, root, "establish-baseline.ps1")
	if !strings.Contains(establish, "'schema', 'fmt'") {
		t.Fatal("establish baseline must validate inspected HCL with offline Atlas schema fmt")
	}
	if strings.Contains(establish, "'schema', 'inspect', '--url', 'file:///runtime/admin.hcl'") {
		t.Fatal("file schema inspect requires a dev database and cannot be used as offline HCL validation")
	}
}

func TestDriftCheckUsesOnlyAtlasOSSFeatures(t *testing.T) {
	root := backendRoot(t)
	drift := mustReadBaselineScript(t, root, "check-drift.ps1")
	for _, proOnly := range []string{"--lock-name", "--lock-timeout"} {
		if strings.Contains(drift, proOnly) {
			t.Fatalf("drift checker uses Atlas Pro-only flag %q", proOnly)
		}
	}
}

func TestDriftCheckRebindsCanonicalSchemaToDisposableDatabase(t *testing.T) {
	root := backendRoot(t)
	drift := mustReadBaselineScript(t, root, "check-drift.ps1")
	for _, required := range []string{
		`schema "admin"`, `schema.$emptyDatabase`, `file:///runtime/admin.hcl`,
	} {
		if !strings.Contains(drift, required) {
			t.Fatalf("drift checker does not rebind canonical schema: missing %q", required)
		}
	}
	if strings.Contains(drift, "--to', 'file:///workspace/database/schema/admin.hcl") {
		t.Fatal("drift checker applies canonical admin schema without rebinding its database name")
	}
	if strings.Contains(drift, ".Contains('schema.admin')") {
		t.Fatal("prefix matching misclassifies schema.admin_empty_* as an unrebound admin reference")
	}
	if !strings.Contains(drift, `\bschema\.admin\b`) {
		t.Fatal("drift checker must detect only an exact unrebound schema.admin reference")
	}
}

func mustReadBaselineScript(t *testing.T, root string, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "scripts", "database", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}
