package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestExpandedSchemaVerifierReportsLegacyGroupsWithoutMutatingThem(t *testing.T) {
	root := backendRoot(t)
	platformSQL, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "034_verify_platform.sql"))
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := os.ReadFile(filepath.Join(root, "scripts", "database", "verify-expanded-schema.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	platformBody := strings.ToLower(string(platformSQL))
	for _, required := range []string{
		"unknown_platform_values", "duplicate_durable_identities", "active_ownership_missing",
		"notifications", "export_tasks", "ai_runs", "payment_callback_events", "wallet_transactions",
	} {
		if !strings.Contains(platformBody, required) {
			t.Errorf("platform invariants missing %q", required)
		}
	}
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bupdate\b`),
		regexp.MustCompile(`(?i)\bdelete\s+from\b`),
		regexp.MustCompile(`(?i)\bdrop\s+table\b`),
	} {
		if forbidden.Match(platformSQL) {
			t.Errorf("platform verifier contains mutation %q", forbidden)
		}
	}
	verifierBody := strings.ToLower(string(verifier))
	for _, required := range []string{
		"030_verify_schema.sql", "031_verify_relations.sql", "032_verify_money.sql", "033_verify_ai.sql", "034_verify_platform.sql",
		"schema_sha256", "legacy_missing_permission_grants", "unresolved_export_object_keys",
		"admin_smoke", "passed",
	} {
		if !strings.Contains(verifierBody, required) {
			t.Errorf("expanded verifier missing %q", required)
		}
	}
}
